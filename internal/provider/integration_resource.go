package provider

import (
	"context"

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

const integrationHost = "integration"

// IntegrationResourceModel is a spotlight_integration as Terraform holds it.
type IntegrationResourceModel struct {
	ID         types.String  `tfsdk:"id"`
	Version    types.Int64   `tfsdk:"version"`
	Address    types.String  `tfsdk:"address"`
	TeamID     types.String  `tfsdk:"team_id"`
	Source     types.String  `tfsdk:"source"`
	Definition types.Dynamic `tfsdk:"definition"`
}

var integrationFields = fieldpath.Locator{Schema: integrationSchema().Type(), Under: fieldpath.Path{fieldpath.Key(expressions.Attribute)}}

func (r *IntegrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integration"
}

func (r *IntegrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = integrationSchema()
}

func integrationSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "An integration a team has defined: how Spotlight hears from a host, and the actions and tools it offers against it. " +
			"The definition is held whole, exactly as the integration's definition is written, with its keys verbatim; write each expression as `$${{ ... }}` so Terraform leaves it alone. " +
			"`terraform plan` type-checks every expression in it where it is written. Import it as `<workspace id>/<team key>/<source>`.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute("integration"),
			"version": versionAttribute("integration"),
			"address": addressAttribute("core/linear"),
			"team_id": ownerAttribute("team", "integration"),
			"source": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The host the integration defines, from `definition.integration`. It is fixed for the integration's life, so changing it replaces the integration.",
			},
			expressions.Attribute: schema.DynamicAttribute{
				Required:            true,
				MarkdownDescription: "The integration's whole definition, as an object. Spotlight fills in each field it leaves out when it reads it back.",
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

// ValidateConfig type-checks the definition's expressions once the provider
// is configured, as it is during a plan.
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

// ModifyPlan plans the source from the definition, and replaces the
// integration when the definition names another host.
func (r *IntegrationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var definition types.Dynamic
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(expressions.Attribute), &definition)...)
	if resp.Diagnostics.HasError() {
		return
	}

	source, known := integrationHostIn(definition)
	if !known {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("source"), source)...)

	if req.State.Raw.IsNull() {
		return
	}

	var prior types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("source"), &prior)...)

	if !prior.IsNull() && prior.ValueString() != source {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root(expressions.Attribute).AtName(integrationHost))
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
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the integration", err, integrationFields, client.ScopeRead)
		return
	}

	kept := &state
	if !answerUnchanged(ctx, req.Private, integrationSettable(integration)) {
		kept = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, integrationModel(integration, kept))...)
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

	held, err := dynamic.ToJSON(plan.Definition)
	if err != nil {
		diags.AddAttributeError(path.Root(expressions.Attribute), "Definition not known", err.Error())

		return nil, diags
	}

	definition, ok := held.(map[string]any)
	if !ok {
		diags.AddAttributeError(path.Root(expressions.Attribute), "Definition is not an object", "Write the definition as an object, as in { integration = \"linear\", ... }.")

		return nil, diags
	}

	return definition, diags
}

func integrationModel(integration client.Integration, kept *IntegrationResourceModel) IntegrationResourceModel {
	model := IntegrationResourceModel{
		ID:         types.StringValue(integration.ID),
		Version:    types.Int64Value(integration.Version),
		Address:    types.StringValue(integration.Address),
		TeamID:     types.StringValue(integration.TeamID),
		Source:     types.StringValue(integration.Source),
		Definition: dynamic.FromJSON(integration.Definition),
	}

	if kept != nil {
		model.Definition = dynamic.Resolve(kept.Definition, integration.Definition)
	}

	return model
}

func integrationHostIn(definition types.Dynamic) (string, bool) {
	if definition.IsNull() || definition.IsUnknown() || definition.IsUnderlyingValueUnknown() {
		return "", false
	}

	object, ok := definition.UnderlyingValue().(basetypes.ObjectValue)
	if !ok {
		return "", false
	}

	named, ok := object.Attributes()[integrationHost].(basetypes.StringValue)
	if !ok || named.IsNull() || named.IsUnknown() {
		return "", false
	}

	return named.ValueString(), true
}
