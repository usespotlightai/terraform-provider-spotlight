package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
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
	_ resource.Resource                   = &RunnerPoolResource{}
	_ resource.ResourceWithConfigure      = &RunnerPoolResource{}
	_ resource.ResourceWithImportState    = &RunnerPoolResource{}
	_ resource.ResourceWithIdentity       = &RunnerPoolResource{}
	_ resource.ResourceWithValidateConfig = &RunnerPoolResource{}
)

// NewRunnerPoolResource returns the spotlight_runner_pool resource.
func NewRunnerPoolResource() resource.Resource {
	return &RunnerPoolResource{}
}

// RunnerPoolResource is the spotlight_runner_pool resource.
type RunnerPoolResource struct {
	client *client.Client
}

const (
	runnerPoolIssuedToken = "Spotlight issued this pool a token, returned only once. Terraform stores no secret in its state, so the token was not stored, and no host can claim with it. " +
		"To use a token that you have instead, set `token` (from a sensitive variable, or a write-only ephemeral value) and change `token_version` to send it."
	runnerPoolPullOnly = "is only for a pool that is pulled from; a pool that provisions is called instead, and has no token."
)

// RunnerPoolResourceModel is a spotlight_runner_pool as Terraform stores it.
type RunnerPoolResourceModel struct {
	ID           types.String         `tfsdk:"id"`
	Version      types.Int64          `tfsdk:"version"`
	Address      types.String         `tfsdk:"address"`
	WorkspaceID  types.String         `tfsdk:"workspace_id"`
	Key          types.String         `tfsdk:"key"`
	Labels       types.Set            `tfsdk:"labels"`
	MaxRuns      types.Int64          `tfsdk:"max_runs"`
	Token        types.String         `tfsdk:"token"`
	TokenVersion types.Int64          `tfsdk:"token_version"`
	Provision    *runnerPoolProvision `tfsdk:"provision"`
}

type runnerPoolProvision struct {
	URL           types.String `tfsdk:"url"`
	Secret        types.String `tfsdk:"secret"`
	SecretVersion types.Int64  `tfsdk:"secret_version"`
	Egress        types.String `tfsdk:"egress"`
	EgressTraffic types.String `tfsdk:"egress_traffic"`
}

var runnerPoolFields = fieldpath.Locator{Schema: runnerPoolSchema().Type()}

func (r *RunnerPoolResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_runner_pool"
}

func (r *RunnerPoolResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = runnerPoolSchema()
}

func runnerPoolSchema() schema.Schema {
	attributes := writeonly.Token.Attributes("The token a `spotlight-host` serving the pool claims work with: 32 to 255 URL-safe characters, as a generated password would be. " +
		"Only for a pool without `provision`. Spotlight stores only its hash. Left out, Spotlight issues one when it creates the pool, which Terraform does not store; " +
		"a change to `token_version` must come with a token. Each token is single-use. Spotlight does not accept the token of another pool. " +
		"A token that you rotated away, or that a deleted or replaced pool used, was used once, so no pool accepts it again, the same pool included. " +
		"Thus, to rotate or replace a pool, you always need a new token.")
	attributes["id"] = idAttribute("runner pool")
	attributes["version"] = versionAttribute("runner pool")
	attributes["address"] = addressAttribute("ephemeral")
	attributes["workspace_id"] = ownerAttribute("workspace", "runner pool")
	attributes["key"] = immutableAttribute("Names the pool, as in `ephemeral`. Placement ignores it: a line or station places its work on the pool by its `labels`.", "runner pool")
	attributes["labels"] = stringSetAttribute("What a line or station may name in `runners` to place its work on the pool's runners, as in `linux`.")
	attributes["max_runs"] = schema.Int64Attribute{
		Optional:            true,
		MarkdownDescription: "The most runs the pool serves at once, from 1 to 10000. Left out, there is no limit.",
	}

	provision := writeonly.ProvisionSecret.Attributes("The secret Spotlight signs each call with, as Standard Webhooks sign: `whsec_` followed by 24 to 64 base64-encoded random bytes, " +
		"as `\"whsec_${base64encode(random_password.provision.result)}\"` makes.")
	provision["url"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Where Spotlight calls, over https, with each run the pool is to serve, as in `https://provision.example.com/runs`.",
	}
	provision["egress"] = schema.StringAttribute{
		Optional: true,
		MarkdownDescription: "The egress the pool's runners send their traffic through: `" + client.PlatformEgress + "` for the egress that Spotlight operates, " +
			"or the `id` of a `spotlight_egress` in the workspace. Left out, the runners have no egress, and Spotlight does not send to the pool a run that uses secrets or the Spotlight API. " +
			"Changing it updates the pool in place.",
	}
	provision["egress_traffic"] = schema.StringAttribute{
		Optional: true,
		Computed: true,
		Default:  stringdefault.StaticString(client.EgressTrafficAll),
		MarkdownDescription: "Which traffic of a run goes through the egress: `" + client.EgressTrafficAll + "`, so egress records every connection and its allow-list applies to every host, " +
			"or `" + client.EgressTrafficBound + "`, only the traffic to hosts that a secret of the run is bound to. Other traffic then goes directly. Only with `egress`.",
		Validators: []validator.String{stringvalidator.OneOf(client.EgressTrafficAll, client.EgressTrafficBound)},
	}
	attributes["provision"] = schema.SingleNestedAttribute{
		Optional: true,
		MarkdownDescription: "Makes the pool one Spotlight pushes to: it calls `url` to start a runner for each run, and the pool has no token. " +
			"Left out, a `spotlight-host` pulls work from the pool with its token. Which of the two a pool does is immutable, so adding or removing `provision` replaces the pool.",
		Attributes:    provision,
		PlanModifiers: []planmodifier.Object{objectplanmodifier.RequiresReplaceIf(provisionAddedOrRemoved, "Adding or removing provision replaces the pool.", "Adding or removing `provision` replaces the pool.")},
	}

	return schema.Schema{
		MarkdownDescription: "A pool of one-run runners: each run the pool serves gets a fresh runner of its own that ends with it. " +
			"A `spotlight-host` on any machine pulls work from the pool with its token and starts a fresh sandbox for each run; " +
			"install one with `curl -fsSL <spotlight>/api/install.sh | SPOTLIGHT_URL=<spotlight> SPOTLIGHT_POOL_TOKEN=<pool token> sh`. " +
			"With `provision`, Spotlight pushes each run to a URL instead, whose call hands a one-run runner its credential. " +
			"Writing runner pools needs a token granted the `runners` scope. Import it as `<workspace id>/<key>`.",
		Attributes: attributes,
	}
}

func provisionAddedOrRemoved(_ context.Context, req planmodifier.ObjectRequest, resp *objectplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = req.StateValue.IsNull() != req.PlanValue.IsNull()
}

func (r *RunnerPoolResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("runner pool")
}

func (r *RunnerPoolResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

// ValidateConfig rejects a token for a push pool, because Spotlight calls the
// provision URL of a push pool.
func (r *RunnerPoolResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var provision types.Object
	var token types.String
	var tokenVersion types.Int64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("provision"), &provision)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, writeonly.Token.Path(), &token)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(writeonly.Token.Version), &tokenVersion)...)

	if provision.IsNull() || provision.IsUnknown() {
		return
	}

	if !token.IsNull() {
		resp.Diagnostics.AddAttributeError(writeonly.Token.Path(), "A pool that provisions takes no token",
			writeonly.Token.Name+": "+runnerPoolPullOnly+" Leave token out, or leave provision out for a pool its runners pull from.")
	}

	if !tokenVersion.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root(writeonly.Token.Version), "A pool that provisions takes no token",
			writeonly.Token.Version+": "+runnerPoolPullOnly+" Leave token_version out, or leave provision out for a pool its runners pull from.")
	}
}

func (r *RunnerPoolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RunnerPoolResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := runnerPoolInput(plan)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(sendRunnerPoolSecrets(ctx, req.Config, req.Plan, nil, &in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pool, err := r.client.CreateRunnerPool(ctx, plan.WorkspaceID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the runner pool", err, runnerPoolFields, client.ScopeRunners)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "runner pool", pool.Warnings, runnerPoolFields)
	resp.Diagnostics.Append(runnerPoolCreated(in)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, runnerPoolModel(pool, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, pool.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, runnerPoolSettable(pool))...)
}

func (r *RunnerPoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RunnerPoolResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pool, err := r.client.ReadRunnerPool(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the runner pool", err, runnerPoolFields, client.ScopeRead)
		return
	}

	prior := &state
	if !answerUnchanged(ctx, req.Private, runnerPoolSettable(pool)) {
		prior = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, runnerPoolModel(pool, prior))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, pool.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, runnerPoolSettable(pool))...)
}

func (r *RunnerPoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state RunnerPoolResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := runnerPoolInput(plan)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(sendRunnerPoolSecrets(ctx, req.Config, req.Plan, &req.State, &in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pool, err := r.client.UpdateRunnerPool(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the runner pool", err, runnerPoolFields, client.ScopeRunners)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "runner pool", pool.Warnings, runnerPoolFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, runnerPoolModel(pool, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, pool.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, runnerPoolSettable(pool))...)
}

func (r *RunnerPoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RunnerPoolResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteRunnerPool(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the runner pool", err, runnerPoolFields, client.ScopeRunners)
	}
}

func (r *RunnerPoolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/ephemeral")
	if !ok {
		return
	}

	var pool client.RunnerPool
	var err error
	if target.byID() {
		pool, err = r.client.ReadRunnerPool(ctx, target.id)
	} else {
		pool, err = r.client.ImportRunnerPool(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the runner pool at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, runnerPoolModel(pool, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, pool.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, runnerPoolSettable(pool))...)
}

func runnerPoolSettable(pool client.RunnerPool) any {
	return pool.RunnerPoolInput
}

func runnerPoolInput(plan RunnerPoolResourceModel) (client.RunnerPoolInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	in := client.RunnerPoolInput{
		Key:          plan.Key.ValueString(),
		Labels:       stringsIn(plan.Labels, &diags),
		MaxRuns:      plan.MaxRuns.ValueInt64Pointer(),
		TokenVersion: plan.TokenVersion.ValueInt64Pointer(),
	}

	if plan.Provision != nil {
		in.Provision = &client.PoolProvision{
			URL:           plan.Provision.URL.ValueString(),
			SecretVersion: plan.Provision.SecretVersion.ValueInt64Pointer(),
			Egress:        plan.Provision.Egress.ValueStringPointer(),
		}
		if !plan.Provision.Egress.IsNull() {
			in.Provision.EgressTraffic = plan.Provision.EgressTraffic.ValueStringPointer()
		}
	}

	return in, diags
}

func sendRunnerPoolSecrets(ctx context.Context, config tfsdk.Config, plan tfsdk.Plan, state *tfsdk.State, in *client.RunnerPoolInput) diag.Diagnostics {
	field := writeonly.Token
	if in.Provision != nil {
		field = writeonly.ProvisionSecret
	}

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

	if value.IsNull() || value.IsUnknown() {
		return diags
	}

	if in.Provision != nil {
		in.Provision.Secret = value.ValueString()
	} else {
		in.Token = value.ValueString()
	}

	return diags
}

func runnerPoolCreated(in client.RunnerPoolInput) diag.Diagnostics {
	var diags diag.Diagnostics

	if in.Provision == nil && in.Token == "" {
		diags.AddAttributeWarning(writeonly.Token.Path(), "Spotlight issued the pool a token Terraform did not store", runnerPoolIssuedToken)
	}

	return diags
}

func runnerPoolModel(pool client.RunnerPool, prior *RunnerPoolResourceModel) RunnerPoolResourceModel {
	model := RunnerPoolResourceModel{
		ID:           types.StringValue(pool.ID),
		Version:      types.Int64Value(pool.Version),
		Address:      types.StringValue(pool.Address),
		WorkspaceID:  types.StringValue(pool.WorkspaceID),
		Key:          types.StringValue(pool.Key),
		Labels:       stringSet(pool.Labels),
		MaxRuns:      types.Int64PointerValue(pool.MaxRuns),
		Token:        types.StringNull(),
		TokenVersion: types.Int64PointerValue(pool.TokenVersion),
	}

	if pool.Provision != nil {
		model.Provision = &runnerPoolProvision{
			URL:           types.StringValue(pool.Provision.URL),
			Secret:        types.StringNull(),
			SecretVersion: types.Int64PointerValue(pool.Provision.SecretVersion),
			Egress:        types.StringPointerValue(pool.Provision.Egress),
			EgressTraffic: types.StringPointerValue(pool.Provision.EgressTraffic),
		}
	}

	if prior != nil {
		model.Key = prior.Key
		model.Labels = prior.Labels
		model.MaxRuns = prior.MaxRuns
		model.TokenVersion = prior.TokenVersion

		if model.Provision != nil && prior.Provision != nil {
			model.Provision.URL = prior.Provision.URL
			model.Provision.SecretVersion = prior.Provision.SecretVersion
			model.Provision.Egress = prior.Provision.Egress
			model.Provision.EgressTraffic = prior.Provision.EgressTraffic
		}
	}

	return model
}
