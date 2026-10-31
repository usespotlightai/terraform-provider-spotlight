package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

var (
	_ resource.Resource                = &OidcTrustResource{}
	_ resource.ResourceWithConfigure   = &OidcTrustResource{}
	_ resource.ResourceWithImportState = &OidcTrustResource{}
	_ resource.ResourceWithIdentity    = &OidcTrustResource{}
)

// NewOidcTrustResource returns the spotlight_oidc_trust
// resource.
func NewOidcTrustResource() resource.Resource {
	return &OidcTrustResource{}
}

// OidcTrustResource is the spotlight_oidc_trust
// resource.
type OidcTrustResource struct {
	client *client.Client
}

// OidcTrustResourceModel is a spotlight_oidc_trust as
// Terraform stores it.
type OidcTrustResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Version         types.Int64  `tfsdk:"version"`
	Address         types.String `tfsdk:"address"`
	WorkspaceID     types.String `tfsdk:"workspace_id"`
	Key             types.String `tfsdk:"key"`
	Issuer          types.String `tfsdk:"issuer"`
	Audience        types.String `tfsdk:"audience"`
	Subject         types.String `tfsdk:"subject"`
	Claims          types.Map    `tfsdk:"claims"`
	Scopes          types.Set    `tfsdk:"scopes"`
	LifetimeMinutes types.Int64  `tfsdk:"lifetime_minutes"`
	CreatedBy       types.String `tfsdk:"created_by"`
}

var oidcTrustFields = fieldpath.Locator{Schema: oidcTrustSchema().Type()}

func (r *OidcTrustResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oidc_trust"
}

func (r *OidcTrustResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = oidcTrustSchema()
}

func oidcTrustSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "A CI workload whose OIDC tokens are exchanged for short-lived workspace tokens, so the CI platform stores no Spotlight token: " +
			"a job whose token the trust matches calls `POST /api/v1/workspace-tokens/exchange`, as the provider's `oidc` block does, and receives a token with the trust's scopes for its lifetime. " +
			"Changing or deleting the trust stops every token that it issued at once. " +
			"Writing OIDC trusts needs a workspace admin, or a token granted the `trusts` scope that has every scope the trust grants. " +
			"A token can grant a trust only the scopes that it has. A token can change a trust only when it has the current scopes and the new scopes of the trust. Import it as `<workspace id>/<key>`.",
		Attributes: map[string]schema.Attribute{
			"id":           idAttribute("OIDC trust"),
			"version":      versionAttribute("OIDC trust"),
			"address":      addressAttribute("github-main"),
			"workspace_id": ownerAttribute("workspace", "OIDC trust"),
			"key":          immutableAttribute("Names the trust, and every token it issues, as in `github-main`.", "OIDC trust"),
			"issuer": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The https URL of the OIDC issuer whose signed tokens the trust accepts, as in `https://token.actions.githubusercontent.com`; its keys are found through OIDC discovery.",
			},
			"audience": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(client.DefaultAudience),
				MarkdownDescription: "The audience the workload must request from its issuer.",
			},
			"subject": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The subject the workload's token must carry, as in `repo:acme/platform:ref:refs/heads/main`. " +
					"`*` matches any run of characters, colons included, as in `repo:acme/platform:*`; a subject with one names its owner before the first, " +
					"so `repo:acme/*` is accepted but `repo:*` and `*` are rejected.",
			},
			"claims": schema.MapAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             mapdefault.StaticValue(stringMap(nil)),
				MarkdownDescription: "Further claims the workload's token must carry, by name, each matching its pattern the way `subject` does, as in `{ repository = \"acme/platform\" }`.",
			},
			"scopes": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.OneOf(
						string(client.ScopeRead), string(client.ScopePlan), string(client.ScopeConfigure), string(client.ScopeSecrets), string(client.ScopeRunners),
						string(client.ScopeTrusts),
					)),
				},
				MarkdownDescription: "The scopes of every token the trust issues: one of `read`, `plan` or `configure`, which are a ladder, and `secrets`, `runners` or `trusts` beside it, as a stored token's are. " +
					"A trust that grants `trusts` lets the workloads it matches write every trust in the workspace, their own included, and so issue tokens with any scope.",
			},
			"lifetime_minutes": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(15),
				Validators:          []validator.Int64{int64validator.Between(1, 60)},
				MarkdownDescription: "How many minutes a token the trust issues stays valid, from 1 to 60.",
			},
			"created_by": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The admin who created the trust; the tokens it issues act on their behalf.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *OidcTrustResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("OIDC trust")
}

func (r *OidcTrustResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *OidcTrustResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OidcTrustResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := oidcTrustInput(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	trust, err := r.client.CreateOidcTrust(ctx, plan.WorkspaceID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the OIDC trust", err, oidcTrustFields, client.ScopeTrusts)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "OIDC trust", trust.Warnings, oidcTrustFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, oidcTrustModel(trust, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, trust.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, oidcTrustSettable(trust))...)
}

func (r *OidcTrustResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OidcTrustResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	trust, err := r.client.ReadOidcTrust(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the OIDC trust", err, oidcTrustFields, client.ScopeRead)
		return
	}

	prior := &state
	if !answerUnchanged(ctx, req.Private, oidcTrustSettable(trust)) {
		prior = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, oidcTrustModel(trust, prior))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, trust.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, oidcTrustSettable(trust))...)
}

func (r *OidcTrustResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state OidcTrustResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := oidcTrustInput(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	trust, err := r.client.UpdateOidcTrust(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the OIDC trust", err, oidcTrustFields, client.ScopeTrusts)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "OIDC trust", trust.Warnings, oidcTrustFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, oidcTrustModel(trust, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, trust.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, oidcTrustSettable(trust))...)
}

func (r *OidcTrustResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state OidcTrustResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteOidcTrust(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the OIDC trust", err, oidcTrustFields, client.ScopeTrusts)
	}
}

func (r *OidcTrustResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/github-main")
	if !ok {
		return
	}

	var trust client.OidcTrust
	var err error
	if target.byID() {
		trust, err = r.client.ReadOidcTrust(ctx, target.id)
	} else {
		trust, err = r.client.ImportOidcTrust(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the OIDC trust at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, oidcTrustModel(trust, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, trust.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, oidcTrustSettable(trust))...)
}

func oidcTrustSettable(trust client.OidcTrust) any {
	return trust.OidcTrustInput
}

func oidcTrustInput(ctx context.Context, plan OidcTrustResourceModel) (client.OidcTrustInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	claims := map[string]string{}
	if !plan.Claims.IsNull() && !plan.Claims.IsUnknown() {
		diags.Append(plan.Claims.ElementsAs(ctx, &claims, false)...)
	}

	return client.OidcTrustInput{
		Key:             plan.Key.ValueString(),
		Issuer:          plan.Issuer.ValueString(),
		Audience:        plan.Audience.ValueString(),
		Subject:         plan.Subject.ValueString(),
		Claims:          claims,
		Scopes:          stringsIn(plan.Scopes, &diags),
		LifetimeMinutes: plan.LifetimeMinutes.ValueInt64(),
	}, diags
}

func oidcTrustModel(trust client.OidcTrust, prior *OidcTrustResourceModel) OidcTrustResourceModel {
	model := OidcTrustResourceModel{
		ID:              types.StringValue(trust.ID),
		Version:         types.Int64Value(trust.Version),
		Address:         types.StringValue(trust.Address),
		WorkspaceID:     types.StringValue(trust.WorkspaceID),
		Key:             types.StringValue(trust.Key),
		Issuer:          types.StringValue(trust.Issuer),
		Audience:        types.StringValue(trust.Audience),
		Subject:         types.StringValue(trust.Subject),
		Claims:          stringMap(trust.Claims),
		Scopes:          stringSet(trust.Scopes),
		LifetimeMinutes: types.Int64Value(trust.LifetimeMinutes),
		CreatedBy:       types.StringValue(trust.CreatedBy),
	}

	if prior != nil {
		model.Key = prior.Key
		model.Issuer = prior.Issuer
		model.Audience = prior.Audience
		model.Subject = prior.Subject
		model.Claims = prior.Claims
		model.Scopes = prior.Scopes
		model.LifetimeMinutes = prior.LifetimeMinutes
	}

	return model
}
