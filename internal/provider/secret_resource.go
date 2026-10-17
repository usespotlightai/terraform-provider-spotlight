package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/writeonly"
)

var (
	_ resource.Resource                   = &SecretResource{}
	_ resource.ResourceWithConfigure      = &SecretResource{}
	_ resource.ResourceWithImportState    = &SecretResource{}
	_ resource.ResourceWithIdentity       = &SecretResource{}
	_ resource.ResourceWithValidateConfig = &SecretResource{}
)

// NewSecretResource returns the spotlight_secret resource.
func NewSecretResource() resource.Resource {
	return &SecretResource{}
}

// SecretResource is the spotlight_secret resource.
type SecretResource struct {
	client *client.Client
}

// SecretResourceModel is a spotlight_secret as Terraform stores it.
type SecretResourceModel struct {
	ID           types.String    `tfsdk:"id"`
	Version      types.Int64     `tfsdk:"version"`
	Address      types.String    `tfsdk:"address"`
	WorkspaceID  types.String    `tfsdk:"workspace_id"`
	Name         types.String    `tfsdk:"name"`
	Source       secretSource    `tfsdk:"source"`
	Bindings     []secretBinding `tfsdk:"bindings"`
	Value        types.String    `tfsdk:"value_wo"`
	ValueVersion types.Int64     `tfsdk:"value_wo_version"`
	RotatedAt    types.String    `tfsdk:"rotated_at"`
}

type secretSource struct {
	Kind     types.String `tfsdk:"kind"`
	SecretID types.String `tfsdk:"secret_id"`
	Field    types.String `tfsdk:"field"`
}

type secretBinding struct {
	Host     types.String `tfsdk:"host"`
	Location types.String `tfsdk:"location"`
	Name     types.String `tfsdk:"name"`
	Prefix   types.String `tfsdk:"prefix"`
}

var (
	secretSourceType = map[string]attr.Type{
		"kind":      types.StringType,
		"secret_id": types.StringType,
		"field":     types.StringType,
	}
	secretBindingType = types.ObjectType{AttrTypes: map[string]attr.Type{
		"host":     types.StringType,
		"location": types.StringType,
		"name":     types.StringType,
		"prefix":   types.StringType,
	}}
)

const (
	locationHeader = "header"
	locationQuery  = "query"
)

var secretFields = fieldpath.Locator{Schema: secretSchema().Type()}

func (r *SecretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (r *SecretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = secretSchema()
}

func secretSchema() schema.Schema {
	attributes := writeonly.Value.Attributes("The value Spotlight stores, usually read from a sensitive variable. " +
		"A secret read from AWS Secrets Manager takes none.")
	attributes["id"] = idAttribute("secret")
	attributes["version"] = versionAttribute("secret")
	attributes["address"] = addressAttribute("LINEAR_API_KEY")
	attributes["workspace_id"] = ownerAttribute("workspace", "secret")
	attributes["name"] = fixedAttribute("What integrations call the secret, as in `LINEAR_API_KEY`, written `secret.LINEAR_API_KEY` where an integration or a harness profile names it.", "secret")
	attributes["source"] = schema.SingleNestedAttribute{
		Optional:            true,
		Computed:            true,
		Default:             objectdefault.StaticValue(types.ObjectValueMust(secretSourceType, map[string]attr.Value{"kind": types.StringValue(client.SourceBuiltIn), "secret_id": types.StringNull(), "field": types.StringNull()})),
		MarkdownDescription: "Where the value lives: stored by Spotlight (`built-in`, the default) or read from AWS Secrets Manager whenever it is used.",
		Attributes: map[string]schema.Attribute{
			"kind": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`built-in` or `aws-secrets-manager`.",
			},
			"secret_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The name or ARN of the secret in AWS Secrets Manager.",
			},
			"field": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The key to read when the AWS secret contains JSON.",
			},
		},
	}
	attributes["bindings"] = schema.ListNestedAttribute{
		Optional:            true,
		Computed:            true,
		Default:             listdefault.StaticValue(types.ListValueMust(secretBindingType, []attr.Value{})),
		MarkdownDescription: "The only hosts the secret is ever sent to, and where Spotlight injects it there; a request anywhere else goes without it.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"host": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: "The host, as in `api.linear.app`, with no scheme, port or path. The secret goes there over HTTPS only.",
				},
				"location": schema.StringAttribute{
					Optional:            true,
					Computed:            true,
					Default:             stringdefault.StaticString(locationHeader),
					MarkdownDescription: "Whether Spotlight injects the secret in a request header or in a query parameter: `header` (the default) or `query`.",
					Validators:          []validator.String{stringvalidator.OneOf(locationHeader, locationQuery)},
				},
				"name": schema.StringAttribute{
					Optional:            true,
					Computed:            true,
					MarkdownDescription: "The header or query parameter that Spotlight injects the secret in. A header defaults to `authorization`; Spotlight stores a header name in lower case. A query parameter must be named.",
					Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					PlanModifiers:       []planmodifier.String{headerNamedByDefault{}},
				},
				"prefix": schema.StringAttribute{
					Optional:            true,
					Computed:            true,
					Default:             stringdefault.StaticString(""),
					MarkdownDescription: "The text that Spotlight writes before the value, as in `Bearer `.",
				},
			},
		},
	}
	attributes["rotated_at"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "When Spotlight last took a new value.",
	}

	return schema.Schema{
		MarkdownDescription: "A secret that Spotlight injects into requests, for the hosts it is bound to. Spotlight stores its value and never answers it, so the value is write-only and never enters the plan or the state. " +
			"Writing secrets needs a token granted the `secrets` scope. Import it as `<workspace id>/<name>`.",
		Attributes: attributes,
	}
}

func (r *SecretResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("secret")
}

func (r *SecretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

// ValidateConfig rejects a value for a secret that Spotlight reads from AWS
// Secrets Manager, because AWS Secrets Manager stores the value. It also rejects
// a binding in a query parameter without a name. The plan checks values that
// are not known yet.
func (r *SecretResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var kind, value types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("source").AtName("kind"), &kind)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(writeonly.Value.Name), &value)...)

	if kind.ValueString() == client.SourceAWSSecretsManager && !value.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root(writeonly.Value.Name), "A secret read from AWS takes no value",
			"value_wo: is stored only by Spotlight; a secret read from AWS Secrets Manager is set there. Leave value_wo out, or store the secret in Spotlight with source.kind = \"built-in\".")
	}

	var listed types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("bindings"), &listed)...)
	if listed.IsNull() || listed.IsUnknown() {
		return
	}

	for at, element := range listed.Elements() {
		binding, ok := element.(types.Object)
		if !ok || binding.IsNull() || binding.IsUnknown() {
			continue
		}

		location, _ := binding.Attributes()["location"].(types.String)
		name, _ := binding.Attributes()["name"].(types.String)
		if location.IsUnknown() || name.IsUnknown() {
			continue
		}

		if location.ValueString() == locationQuery && name.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("bindings").AtListIndex(at).AtName("name"), "A query parameter must be named",
				"name: is the query parameter that Spotlight injects the secret in, as in key or api_key. Name it, or inject the secret in a header with location = \"header\".")
		}
	}
}

type headerNamedByDefault struct{}

func (headerNamedByDefault) Description(_ context.Context) string {
	return "a header defaults to authorization"
}

func (m headerNamedByDefault) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (headerNamedByDefault) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if !req.ConfigValue.IsNull() || !req.PlanValue.IsUnknown() {
		return
	}

	var location types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, req.Path.ParentPath().AtName("location"), &location)...)

	if location.ValueString() == locationHeader {
		resp.PlanValue = types.StringValue("authorization")
	}
}

func (r *SecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SecretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := secretInput(plan)
	resp.Diagnostics.Append(sendSecretValue(ctx, req.Config, req.Plan, nil, &in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, err := r.client.CreateSecret(ctx, plan.WorkspaceID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the secret", err, secretFields, client.ScopeSecrets)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "secret", secret.Warnings, secretFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, secretModel(secret, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, secret.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, secretSettable(secret))...)
}

func (r *SecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, err := r.client.ReadSecret(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the secret", err, secretFields, client.ScopeRead)
		return
	}

	prior := &state
	if !answerUnchanged(ctx, req.Private, secretSettable(secret)) {
		prior = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, secretModel(secret, prior))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, secret.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, secretSettable(secret))...)
}

func (r *SecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SecretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := secretInput(plan)
	resp.Diagnostics.Append(sendSecretValue(ctx, req.Config, req.Plan, &req.State, &in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, err := r.client.UpdateSecret(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the secret", err, secretFields, client.ScopeSecrets)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "secret", secret.Warnings, secretFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, secretModel(secret, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, secret.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, secretSettable(secret))...)
}

func (r *SecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteSecret(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the secret", err, secretFields, client.ScopeSecrets)
	}
}

func (r *SecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/LINEAR_API_KEY")
	if !ok {
		return
	}

	var secret client.Secret
	var err error
	if target.byID() {
		secret, err = r.client.ReadSecret(ctx, target.id)
	} else {
		secret, err = r.client.ImportSecret(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the secret at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, secretModel(secret, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, secret.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, secretSettable(secret))...)
}

func secretInput(plan SecretResourceModel) client.SecretInput {
	in := client.SecretInput{
		Name: plan.Name.ValueString(),
		Source: client.SecretSource{
			Kind:     plan.Source.Kind.ValueString(),
			SecretID: plan.Source.SecretID.ValueString(),
			Field:    plan.Source.Field.ValueString(),
		},
		Bindings: make([]client.SecretBinding, 0, len(plan.Bindings)),
	}

	for _, each := range plan.Bindings {
		in.Bindings = append(in.Bindings, client.SecretBinding{
			Host:     each.Host.ValueString(),
			Location: each.Location.ValueString(),
			Name:     each.Name.ValueString(),
			Prefix:   each.Prefix.ValueString(),
		})
	}

	if !plan.ValueVersion.IsNull() && !plan.ValueVersion.IsUnknown() {
		version := plan.ValueVersion.ValueInt64()
		in.ValueVersion = &version
	}

	return in
}

func sendSecretValue(ctx context.Context, config tfsdk.Config, plan tfsdk.Plan, state *tfsdk.State, in *client.SecretInput) diag.Diagnostics {
	var diags diag.Diagnostics

	if state != nil {
		rewritten, more := writeonly.Value.Rewritten(ctx, plan, *state)
		diags.Append(more...)

		storedFromNow, more := secretBecomesBuiltIn(ctx, *state, in.Source)
		diags.Append(more...)

		if !rewritten && !storedFromNow {
			return diags
		}
	}

	value, more := writeonly.Value.Read(ctx, config)
	diags.Append(more...)

	if !value.IsNull() && !value.IsUnknown() {
		text := value.ValueString()
		in.Value = &text
	}

	return diags
}

func secretBecomesBuiltIn(ctx context.Context, state tfsdk.State, planned client.SecretSource) (bool, diag.Diagnostics) {
	var prior types.String
	diags := state.GetAttribute(ctx, path.Root("source").AtName("kind"), &prior)

	return planned.Kind == client.SourceBuiltIn && prior.ValueString() != client.SourceBuiltIn, diags
}

func secretSettable(s client.Secret) any {
	return map[string]any{
		"name":           s.Name,
		"source":         s.Source,
		"bindings":       s.Bindings,
		"valueWoVersion": s.ValueVersion,
	}
}

func secretModel(secret client.Secret, prior *SecretResourceModel) SecretResourceModel {
	model := SecretResourceModel{
		ID:          types.StringValue(secret.ID),
		Version:     types.Int64Value(secret.Version),
		Address:     types.StringValue(secret.Address),
		WorkspaceID: types.StringValue(secret.WorkspaceID),
		Name:        types.StringValue(secret.Name),
		Source: secretSource{
			Kind:     types.StringValue(secret.Source.Kind),
			SecretID: textOrNull(secret.Source.SecretID),
			Field:    textOrNull(secret.Source.Field),
		},
		Bindings:     make([]secretBinding, 0, len(secret.Bindings)),
		Value:        types.StringNull(),
		ValueVersion: types.Int64PointerValue(secret.ValueVersion),
		RotatedAt:    types.StringValue(secret.RotatedAt),
	}

	for _, each := range secret.Bindings {
		model.Bindings = append(model.Bindings, secretBinding{
			Host:     types.StringValue(each.Host),
			Location: types.StringValue(each.Location),
			Name:     types.StringValue(each.Name),
			Prefix:   types.StringValue(each.Prefix),
		})
	}

	if prior != nil {
		model.Name = prior.Name
		model.Source = prior.Source
		model.Bindings = prior.Bindings
		model.ValueVersion = prior.ValueVersion
	}

	return model
}
