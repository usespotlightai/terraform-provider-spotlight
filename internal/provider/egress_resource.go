package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/writeonly"
)

var (
	_ resource.Resource                = &EgressResource{}
	_ resource.ResourceWithConfigure   = &EgressResource{}
	_ resource.ResourceWithImportState = &EgressResource{}
	_ resource.ResourceWithIdentity    = &EgressResource{}
)

// NewEgressResource returns the spotlight_egress resource.
func NewEgressResource() resource.Resource {
	return &EgressResource{}
}

// EgressResource is the spotlight_egress resource.
type EgressResource struct {
	client *client.Client
}

const egressIssuedToken = "Spotlight issued this egress a token, answered only once. Terraform stores no secret in its state, so the token was not stored, and the egress cannot call Spotlight with it. " +
	"To use a token that you have instead, set `token` (from a sensitive variable, or a write-only ephemeral value) and change `token_version` to send it."

// EgressResourceModel is a spotlight_egress as Terraform stores it.
type EgressResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Version      types.Int64  `tfsdk:"version"`
	Address      types.String `tfsdk:"address"`
	WorkspaceID  types.String `tfsdk:"workspace_id"`
	Key          types.String `tfsdk:"key"`
	URL          types.String `tfsdk:"url"`
	Token        types.String `tfsdk:"token"`
	TokenVersion types.Int64  `tfsdk:"token_version"`
}

var egressFields = fieldpath.Locator{Schema: egressSchema().Type()}

func (r *EgressResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_egress"
}

func (r *EgressResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = egressSchema()
}

func egressSchema() schema.Schema {
	attributes := writeonly.EgressToken.Attributes("The token the egress calls Spotlight with: 32 to 255 URL-safe characters, as a generated password would be. " +
		"Spotlight stores only its hash. Left out, Spotlight issues one when it creates the egress, which Terraform does not store; " +
		"a change to `token_version` must come with a token. Each token is single-use. Spotlight does not accept the token of another egress. " +
		"A token that you rotated away, or that a deleted or replaced egress used, was used once, so no egress accepts it again, the same egress included. " +
		"Thus, to rotate or replace an egress, you always need a new token.")
	attributes["id"] = idAttribute("egress")
	attributes["version"] = versionAttribute("egress")
	attributes["address"] = addressAttribute("shared")
	attributes["workspace_id"] = ownerAttribute("workspace", "egress")
	attributes["key"] = fixedAttribute("Names the egress, as in `shared`.", "egress")
	attributes["url"] = schema.StringAttribute{
		Required: true,
		MarkdownDescription: "Where the runners of a pool that names the egress send their traffic, as in `https://egress.example.com`. " +
			"It has no credentials, no query and no fragment. An http URL gets a warning: the runners send their proxy credential to it, " +
			"and a production deployment does not provision runners that use it.",
	}

	return schema.Schema{
		MarkdownDescription: "An egress that the runners of push pools send their traffic through. A `spotlight_runner_pool` names it in `provision.egress`. " +
			"For each run, the egress adds the secrets of the run to the requests that go to their bound hosts. A bound host is a host that a secret of the run is bound to. " +
			"The egress calls Spotlight with its token. Spotlight does not delete an egress while a pool names it. " +
			"Terraform can delete the egress before it updates the pools, so first remove `provision.egress` from the pools and apply, then remove the egress. " +
			"Writing egresses needs a token granted the `runners` scope. Import it as `<workspace id>/<key>`.",
		Attributes: attributes,
	}
}

func (r *EgressResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("egress")
}

func (r *EgressResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *EgressResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan EgressResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := egressInput(plan)
	resp.Diagnostics.Append(sendEgressToken(ctx, req.Config, req.Plan, nil, &in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	egress, err := r.client.CreateEgress(ctx, plan.WorkspaceID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the egress", err, egressFields, client.ScopeRunners)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "egress", egress.Warnings, egressFields)
	resp.Diagnostics.Append(egressCreated(in)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, egressModel(egress, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, egress.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, egressSettable(egress))...)
}

func (r *EgressResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EgressResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	egress, err := r.client.ReadEgress(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the egress", err, egressFields, client.ScopeRead)
		return
	}

	prior := &state
	if !answerUnchanged(ctx, req.Private, egressSettable(egress)) {
		prior = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, egressModel(egress, prior))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, egress.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, egressSettable(egress))...)
}

func (r *EgressResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state EgressResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := egressInput(plan)
	resp.Diagnostics.Append(sendEgressToken(ctx, req.Config, req.Plan, &req.State, &in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	egress, err := r.client.UpdateEgress(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the egress", err, egressFields, client.ScopeRunners)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "egress", egress.Warnings, egressFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, egressModel(egress, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, egress.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, egressSettable(egress))...)
}

func (r *EgressResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EgressResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteEgress(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the egress", err, egressFields, client.ScopeRunners)
	}
}

func (r *EgressResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/shared")
	if !ok {
		return
	}

	var egress client.Egress
	var err error
	if target.byID() {
		egress, err = r.client.ReadEgress(ctx, target.id)
	} else {
		egress, err = r.client.ImportEgress(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the egress at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, egressModel(egress, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, egress.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, egressSettable(egress))...)
}

func egressSettable(egress client.Egress) any {
	return egress.EgressInput
}

func egressInput(plan EgressResourceModel) client.EgressInput {
	return client.EgressInput{
		Key:          plan.Key.ValueString(),
		URL:          plan.URL.ValueString(),
		TokenVersion: plan.TokenVersion.ValueInt64Pointer(),
	}
}

func sendEgressToken(ctx context.Context, config tfsdk.Config, plan tfsdk.Plan, state *tfsdk.State, in *client.EgressInput) diag.Diagnostics {
	field := writeonly.EgressToken

	var diags diag.Diagnostics

	if state != nil {
		rewritten, more := field.Rewritten(ctx, plan, *state)
		diags.Append(more...)

		if !rewritten {
			return diags
		}
	}

	value, more := field.Read(ctx, config)
	diags.Append(more...)

	if !value.IsNull() && !value.IsUnknown() {
		in.Token = value.ValueString()
	}

	return diags
}

func egressCreated(in client.EgressInput) diag.Diagnostics {
	var diags diag.Diagnostics

	if in.Token == "" {
		diags.AddAttributeWarning(writeonly.EgressToken.Path(), "Spotlight issued the egress a token Terraform did not store", egressIssuedToken)
	}

	return diags
}

func egressModel(egress client.Egress, prior *EgressResourceModel) EgressResourceModel {
	model := EgressResourceModel{
		ID:           types.StringValue(egress.ID),
		Version:      types.Int64Value(egress.Version),
		Address:      types.StringValue(egress.Address),
		WorkspaceID:  types.StringValue(egress.WorkspaceID),
		Key:          types.StringValue(egress.Key),
		URL:          types.StringValue(egress.URL),
		Token:        types.StringNull(),
		TokenVersion: types.Int64PointerValue(egress.TokenVersion),
	}

	if prior != nil {
		model.Key = prior.Key
		model.URL = prior.URL
		model.TokenVersion = prior.TokenVersion
	}

	return model
}
