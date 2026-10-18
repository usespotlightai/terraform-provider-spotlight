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
	_ resource.Resource                = &WorkspaceTokenTrustResource{}
	_ resource.ResourceWithConfigure   = &WorkspaceTokenTrustResource{}
	_ resource.ResourceWithImportState = &WorkspaceTokenTrustResource{}
	_ resource.ResourceWithIdentity    = &WorkspaceTokenTrustResource{}
)

// NewWorkspaceTokenTrustResource returns the spotlight_workspace_token_trust
// resource.
func NewWorkspaceTokenTrustResource() resource.Resource {
	return &WorkspaceTokenTrustResource{}
}

// WorkspaceTokenTrustResource is the spotlight_workspace_token_trust
// resource.
type WorkspaceTokenTrustResource struct {
	client *client.Client
}

// WorkspaceTokenTrustResourceModel is a spotlight_workspace_token_trust as
// Terraform stores it.
type WorkspaceTokenTrustResourceModel struct {
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
	DefinedBy       types.String `tfsdk:"defined_by"`
}

var workspaceTokenTrustFields = fieldpath.Locator{Schema: workspaceTokenTrustSchema().Type()}

func (r *WorkspaceTokenTrustResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workspace_token_trust"
}

func (r *WorkspaceTokenTrustResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = workspaceTokenTrustSchema()
}

func workspaceTokenTrustSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "A CI workload whose OIDC tokens are exchanged for short-lived workspace tokens, so the CI platform stores no Spotlight token: " +
			"a job whose token the trust admits calls `POST /api/workspace-tokens/exchange`, as the provider's `oidc` block does, and is given a token with the trust's scopes for its lifetime. " +
			"Changing or deleting the trust stops every token it minted at once. " +
			"Writing workspace token trusts needs a workspace admin, or a token granted the `trusts` scope that has every scope the trust gives. " +
			"A token can give a trust only the scopes that it has. A token can change a trust only when it has the current scopes and the new scopes of the trust. Import it as `<workspace id>/<key>`.",
		Attributes: map[string]schema.Attribute{
			"id":           idAttribute("workspace token trust"),
			"version":      versionAttribute("workspace token trust"),
			"address":      addressAttribute("github-main"),
			"workspace_id": ownerAttribute("workspace", "workspace token trust"),
			"key":          fixedAttribute("Names the trust, and every token it mints, as in `github-main`.", "workspace token trust"),
			"issuer": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The https URL of the OIDC issuer whose signed tokens the trust accepts, as in `https://token.actions.githubusercontent.com`; its keys are found through OIDC discovery.",
			},
			"audience": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(client.DefaultAudience),
				MarkdownDescription: "The audience the workload must ask its issuer for.",
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
				MarkdownDescription: "The scopes every token the trust mints is given: one of `read`, `plan` or `configure`, which are a ladder, and `secrets`, `runners` or `trusts` beside it, as a stored token's are. " +
					"A trust that grants `trusts` lets the workloads it admits write every trust in the workspace, their own included, and so mint tokens with any scope.",
			},
			"lifetime_minutes": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(15),
				Validators:          []validator.Int64{int64validator.Between(1, 60)},
				MarkdownDescription: "How many minutes a token the trust mints admits its bearer, from 1 to 60.",
			},
			"defined_by": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The admin who defined the trust; the tokens it mints act on their behalf.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *WorkspaceTokenTrustResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("workspace token trust")
}

func (r *WorkspaceTokenTrustResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *WorkspaceTokenTrustResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan WorkspaceTokenTrustResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := workspaceTokenTrustInput(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	trust, err := r.client.CreateWorkspaceTokenTrust(ctx, plan.WorkspaceID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the workspace token trust", err, workspaceTokenTrustFields, client.ScopeTrusts)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "workspace token trust", trust.Warnings, workspaceTokenTrustFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceTokenTrustModel(trust, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, trust.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, workspaceTokenTrustSettable(trust))...)
}

func (r *WorkspaceTokenTrustResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state WorkspaceTokenTrustResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	trust, err := r.client.ReadWorkspaceTokenTrust(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the workspace token trust", err, workspaceTokenTrustFields, client.ScopeRead)
		return
	}

	kept := &state
	if !answerUnchanged(ctx, req.Private, workspaceTokenTrustSettable(trust)) {
		kept = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceTokenTrustModel(trust, kept))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, trust.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, workspaceTokenTrustSettable(trust))...)
}

func (r *WorkspaceTokenTrustResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state WorkspaceTokenTrustResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := workspaceTokenTrustInput(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	trust, err := r.client.UpdateWorkspaceTokenTrust(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the workspace token trust", err, workspaceTokenTrustFields, client.ScopeTrusts)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "workspace token trust", trust.Warnings, workspaceTokenTrustFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceTokenTrustModel(trust, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, trust.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, workspaceTokenTrustSettable(trust))...)
}

func (r *WorkspaceTokenTrustResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state WorkspaceTokenTrustResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteWorkspaceTokenTrust(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the workspace token trust", err, workspaceTokenTrustFields, client.ScopeTrusts)
	}
}

func (r *WorkspaceTokenTrustResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/github-main")
	if !ok {
		return
	}

	var trust client.WorkspaceTokenTrust
	var err error
	if target.byID() {
		trust, err = r.client.ReadWorkspaceTokenTrust(ctx, target.id)
	} else {
		trust, err = r.client.ImportWorkspaceTokenTrust(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the workspace token trust at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceTokenTrustModel(trust, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, trust.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, workspaceTokenTrustSettable(trust))...)
}

func workspaceTokenTrustSettable(trust client.WorkspaceTokenTrust) any {
	return trust.WorkspaceTokenTrustInput
}

func workspaceTokenTrustInput(ctx context.Context, plan WorkspaceTokenTrustResourceModel) (client.WorkspaceTokenTrustInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	claims := map[string]string{}
	if !plan.Claims.IsNull() && !plan.Claims.IsUnknown() {
		diags.Append(plan.Claims.ElementsAs(ctx, &claims, false)...)
	}

	return client.WorkspaceTokenTrustInput{
		Key:             plan.Key.ValueString(),
		Issuer:          plan.Issuer.ValueString(),
		Audience:        plan.Audience.ValueString(),
		Subject:         plan.Subject.ValueString(),
		Claims:          claims,
		Scopes:          stringsIn(plan.Scopes, &diags),
		LifetimeMinutes: plan.LifetimeMinutes.ValueInt64(),
	}, diags
}

func workspaceTokenTrustModel(trust client.WorkspaceTokenTrust, kept *WorkspaceTokenTrustResourceModel) WorkspaceTokenTrustResourceModel {
	model := WorkspaceTokenTrustResourceModel{
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
		DefinedBy:       types.StringValue(trust.DefinedBy),
	}

	if kept != nil {
		model.Key = kept.Key
		model.Issuer = kept.Issuer
		model.Audience = kept.Audience
		model.Subject = kept.Subject
		model.Claims = kept.Claims
		model.Scopes = kept.Scopes
		model.LifetimeMinutes = kept.LifetimeMinutes
	}

	return model
}
