package provider

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/dynamic"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/expressions"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

var (
	_ resource.Resource                   = &IntegrationResource{}
	_ resource.ResourceWithConfigure      = &IntegrationResource{}
	_ resource.ResourceWithImportState    = &IntegrationResource{}
	_ resource.ResourceWithIdentity       = &IntegrationResource{}
	_ resource.ResourceWithValidateConfig = &IntegrationResource{}
	_ resource.ResourceWithModifyPlan     = &IntegrationResource{}
)

// NewIntegrationResource returns the spotlight_integration resource.
func NewIntegrationResource() resource.Resource {
	return &IntegrationResource{}
}

// IntegrationResource is the spotlight_integration resource.
type IntegrationResource struct {
	client      *client.Client
	expressions *expressions.Checker
}

const (
	integrationKey   = "integration"
	integrationTools = "tools"
	integrationSpec  = "spec"
)

var integrationSpecKinds = []string{"openapi", "graphql", "mcp"}

// IntegrationResourceModel is a spotlight_integration as Terraform stores it.
type IntegrationResourceModel struct {
	ID          types.String  `tfsdk:"id"`
	Version     types.Int64   `tfsdk:"version"`
	Address     types.String  `tfsdk:"address"`
	TeamID      types.String  `tfsdk:"team_id"`
	Integration types.String  `tfsdk:"integration"`
	Definition  types.Dynamic `tfsdk:"definition"`
	Tools       types.Map     `tfsdk:"tools"`
	Spec        types.Object  `tfsdk:"spec"`
}

var (
	integrationFields    = fieldpath.Locator{Schema: integrationSchema().Type(), Under: fieldpath.Path{fieldpath.Key(expressions.Attribute)}}
	integrationSpecTypes = attributeTypesAt(integrationSchema().Type(), integrationSpec)
)

func (r *IntegrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integration"
}

func (r *IntegrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = integrationSchema()
}

func integrationSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "An integration a team has created: how Spotlight receives events from a service, and the actions and tools it provides for that service. " +
			"Spotlight stores the definition whole, exactly as the integration's definition is written, with its keys verbatim; write each expression as `$${{ ... }}` so Terraform leaves it alone. " +
			"`terraform plan` type-checks every expression in it where it is written. " +
			"A definition may name an API spec, `openapi` or `graphql`, that Spotlight generates a tool from for each operation when it reads it, or a remote MCP server, `mcp`, whose tools it lists; those tools are known only after apply. " +
			"Import it as `<workspace id>/<team key>/<integration>`.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute("integration"),
			"version": versionAttribute("integration"),
			"address": addressAttribute("core/linear"),
			"team_id": ownerAttribute("team", "integration"),
			integrationKey: schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the integration, from `definition.integration`. It is immutable, so changing it replaces the integration.",
			},
			expressions.Attribute: schema.DynamicAttribute{
				Required:            true,
				MarkdownDescription: "The integration's whole definition, as an object. Spotlight fills in each field it leaves out when it reads it back.",
			},
			integrationTools: schema.MapAttribute{
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Each tool the integration provides, by its name in `definition.tools` or as generated from its API spec or MCP server, to the name that a station or hook uses for it, as in `{ search = \"grafana.search\" }`. " +
					"Name a tool through it, as in `spotlight_integration.grafana.tools[\"search\"]`, so a tool renamed or removed fails the plan where it is named. " +
					"When the definition names an API spec or an MCP server the tools are known only after apply, so a generated tool named through it that is not there fails the apply, not the plan.",
			},
			integrationSpec: schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The API spec or MCP server the integration's tools are generated from, as Spotlight last read it; null when the definition names none.",
				Attributes: map[string]schema.Attribute{
					"kind": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "`openapi`, `graphql` or `mcp`.",
					},
					"hash": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The sha256 of the last good read, unchanged while a later read fails; null when the spec has never been read.",
					},
					"read_at": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "When the spec was last read, in RFC 3339.",
					},
					"problem": schema.StringAttribute{
						Computed: true,
						MarkdownDescription: "Why the latest read failed; null when it succeeded. While it is set, the integration keeps the tools from the last good read. " +
							"Spotlight reads the spec when the integration is created, and again when its `url`, `document`, `baseUrl`, `revision`, `depth` or kind of `auth` changes, " +
							"on any update after a failed read, and when the integration is connected over OAuth. Spotlight lists an MCP server's tools again every ten minutes, or as often as its deployment sets. Calling a tool never reads it.",
					},
					"tools": schema.Int64Attribute{
						Computed:            true,
						MarkdownDescription: "How many of the integration's tools were generated from the spec's last good read.",
					},
				},
			},
		},
	}
}

func (r *IntegrationResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("integration")
}

func (r *IntegrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataOf(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}

	r.client = data.client
	r.expressions = data.expressions
}

// ValidateConfig type-checks the expressions in the definition when the
// provider is configured, as during a plan.
func (r *IntegrationResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	if r.expressions == nil {
		return
	}

	var definition types.Dynamic
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(expressions.Attribute), &definition)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.expressions.Check(ctx, definition)...)
}

// ModifyPlan plans the integration and tools from the definition. It replaces
// the integration when the definition names a different integration. The apply
// sets the tools that come from an API spec.
func (r *IntegrationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var definition types.Dynamic
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(expressions.Attribute), &definition)...)
	if resp.Diagnostics.HasError() {
		return
	}

	named, known := integrationKeyIn(definition)
	if !known {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(integrationKey), named)...)

	if !integrationNamesSpec(definition) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(integrationSpec), types.ObjectNull(integrationSpecTypes))...)
	}

	if tools, known := integrationToolsIn(definition, named); known {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(integrationTools), tools)...)
	}

	if req.State.Raw.IsNull() {
		return
	}

	var prior types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(integrationKey), &prior)...)

	if !prior.IsNull() && prior.ValueString() != named {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root(expressions.Attribute).AtName(integrationKey))
	}
}

func (r *IntegrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IntegrationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := integrationInput(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	integration, err := r.client.CreateIntegration(ctx, plan.TeamID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the integration", err, integrationFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "integration", integration.Warnings, integrationFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationModel(integration, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, integration.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, integrationSettable(integration))...)
}

func (r *IntegrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IntegrationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	integration, err := r.client.ReadIntegration(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the integration", err, integrationFields, client.ScopeRead)
		return
	}

	prior := &state
	if !answerUnchanged(ctx, req.Private, integrationSettable(integration)) {
		prior = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, integrationModel(integration, prior))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, integration.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, integrationSettable(integration))...)
}

func (r *IntegrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state IntegrationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := integrationInput(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	integration, err := r.client.UpdateIntegration(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the integration", err, integrationFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "integration", integration.Warnings, integrationFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationModel(integration, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, integration.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, integrationSettable(integration))...)
}

func (r *IntegrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IntegrationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteIntegration(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the integration", err, integrationFields, client.ScopeConfigure)
	}
}

func (r *IntegrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/core/linear")
	if !ok {
		return
	}

	var integration client.Integration
	var err error
	if target.byID() {
		integration, err = r.client.ReadIntegration(ctx, target.id)
	} else {
		integration, err = r.client.ImportIntegration(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the integration at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, integrationModel(integration, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, integration.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, integrationSettable(integration))...)
}

func integrationSettable(integration client.Integration) any {
	return integration.Definition
}

func integrationInput(plan IntegrationResourceModel) (client.Definition, diag.Diagnostics) {
	var diags diag.Diagnostics

	decoded, err := dynamic.ToJSON(plan.Definition)
	if err != nil {
		diags.AddAttributeError(path.Root(expressions.Attribute), "Definition not known", err.Error())

		return nil, diags
	}

	definition, ok := decoded.(map[string]any)
	if !ok {
		diags.AddAttributeError(path.Root(expressions.Attribute), "Definition is not an object", "Write the definition as an object, as in { integration = \"linear\", ... }.")

		return nil, diags
	}

	return definition, diags
}

func integrationModel(integration client.Integration, prior *IntegrationResourceModel) IntegrationResourceModel {
	model := IntegrationResourceModel{
		ID:          types.StringValue(integration.ID),
		Version:     types.Int64Value(integration.Version),
		Address:     types.StringValue(integration.Address),
		TeamID:      types.StringValue(integration.TeamID),
		Integration: types.StringValue(integration.Integration),
		Definition:  dynamic.FromJSON(integration.Definition),
		Tools:       stringMap(integration.Tools),
		Spec:        integrationSpecValue(integration.Spec),
	}

	if prior != nil {
		model.Definition = dynamic.Resolve(prior.Definition, integration.Definition)
	}

	return model
}

func integrationSpecValue(spec *client.Spec) types.Object {
	if spec == nil {
		return types.ObjectNull(integrationSpecTypes)
	}

	return types.ObjectValueMust(integrationSpecTypes, map[string]attr.Value{
		"kind":    types.StringValue(spec.Kind),
		"hash":    types.StringPointerValue(spec.Hash),
		"read_at": types.StringValue(spec.ReadAt.Format(time.RFC3339Nano)),
		"problem": types.StringPointerValue(spec.Problem),
		"tools":   types.Int64Value(spec.Tools),
	})
}

func integrationKeyIn(definition types.Dynamic) (string, bool) {
	if definition.IsNull() || definition.IsUnknown() || definition.IsUnderlyingValueUnknown() {
		return "", false
	}

	object, ok := definition.UnderlyingValue().(basetypes.ObjectValue)
	if !ok {
		return "", false
	}

	named, ok := object.Attributes()[integrationKey].(basetypes.StringValue)
	if !ok || named.IsNull() || named.IsUnknown() {
		return "", false
	}

	return named.ValueString(), true
}

func integrationNamesSpec(definition types.Dynamic) bool {
	object, ok := definition.UnderlyingValue().(basetypes.ObjectValue)
	if !ok {
		return false
	}

	return slices.ContainsFunc(integrationSpecKinds, func(kind string) bool {
		named, ok := object.Attributes()[kind]

		return ok && !named.IsNull()
	})
}

func integrationToolsIn(definition types.Dynamic, integration string) (types.Map, bool) {
	object, ok := definition.UnderlyingValue().(basetypes.ObjectValue)
	if !ok || integrationNamesSpec(definition) {
		return types.Map{}, false
	}

	var names []string
	switch tools := object.Attributes()[integrationTools].(type) {
	case nil:
	case basetypes.ObjectValue:
		if tools.IsUnknown() {
			return types.Map{}, false
		}
		names = slices.Collect(maps.Keys(tools.Attributes()))
	case basetypes.MapValue:
		if tools.IsUnknown() {
			return types.Map{}, false
		}
		names = slices.Collect(maps.Keys(tools.Elements()))
	case basetypes.DynamicValue:
		if !tools.IsNull() {
			return types.Map{}, false
		}
	default:
		return types.Map{}, false
	}

	provided := make(map[string]string, len(names))
	for _, name := range names {
		provided[name] = integration + "." + name
	}

	return stringMap(provided), true
}
