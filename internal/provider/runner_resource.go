package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/writeonly"
)

var (
	_ resource.Resource                = &RunnerResource{}
	_ resource.ResourceWithConfigure   = &RunnerResource{}
	_ resource.ResourceWithImportState = &RunnerResource{}
	_ resource.ResourceWithIdentity    = &RunnerResource{}
)

// NewRunnerResource returns the spotlight_runner resource.
func NewRunnerResource() resource.Resource {
	return &RunnerResource{}
}

// RunnerResource is the spotlight_runner resource.
type RunnerResource struct {
	client *client.Client
}

const runnerIssuedToken = "Spotlight issued this runner a token, answered only once. Terraform keeps no secret in its state, so the token was not kept, and no runner can call with it. " +
	"Give the runner a token you hold instead: set `token` (from a sensitive variable, or a write-only ephemeral value) and change `token_version` to send it. " +
	"A runner on a platform that issues its workloads tokens can enrol through a `spotlight_runner_trust` rather than hold a token."

// RunnerResourceModel is a spotlight_runner as Terraform holds it.
type RunnerResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Version      types.Int64  `tfsdk:"version"`
	Address      types.String `tfsdk:"address"`
	WorkspaceID  types.String `tfsdk:"workspace_id"`
	Name         types.String `tfsdk:"name"`
	Labels       types.Set    `tfsdk:"labels"`
	Token        types.String `tfsdk:"token"`
	TokenVersion types.Int64  `tfsdk:"token_version"`
	Trust        types.String `tfsdk:"trust"`
}

var runnerFields = fieldpath.Locator{Schema: runnerSchema().Type()}

func (r *RunnerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_runner"
}

func (r *RunnerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = runnerSchema()
}

func runnerSchema() schema.Schema {
	attributes := writeonly.Token.Attributes("The token the runner calls with: 32 to 255 URL-safe characters, as a generated password would be. " +
		"Spotlight keeps only its hash. Left out, Spotlight issues one when it creates the runner, which Terraform does not keep.")
	attributes["id"] = idAttribute("runner")
	attributes["version"] = versionAttribute("runner")
	attributes["address"] = addressAttribute("build-1")
	attributes["workspace_id"] = ownerAttribute("workspace", "runner")
	attributes["name"] = fixedAttribute("What the runner is called, and what a line or station names in `runners` to place its work on it.", "runner")
	attributes["labels"] = stringSetAttribute("What a line or station may name in `runners` to place its work here, beside the runner's name, as in `vpc-internal`.")
	attributes["trust"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The key of the runner trust the runner enrolled through, or null when it holds a token.",
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}

	return schema.Schema{
		MarkdownDescription: "A runner that calls Spotlight with a token, to claim and run its workspace's work. " +
			"Writing runners needs a token granted the `runners` scope. Import it as `<workspace id>/<name>`.",
		Attributes: attributes,
	}
}

func (r *RunnerResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("runner")
}

func (r *RunnerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *RunnerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RunnerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := runnerInput(plan)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(sendRunnerToken(ctx, req.Config, req.Plan, nil, &in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	runner, err := r.client.CreateRunner(ctx, plan.WorkspaceID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the runner", err, runnerFields, client.ScopeRunners)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "runner", runner.Warnings, runnerFields)
	resp.Diagnostics.Append(runnerCreated(runner)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, runnerModel(runner, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, runner.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, runnerSettable(runner))...)
}

func (r *RunnerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RunnerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	runner, err := r.client.ReadRunner(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the runner", err, runnerFields, client.ScopeRead)
		return
	}

	kept := &state
	if !answerUnchanged(ctx, req.Private, runnerSettable(runner)) {
		kept = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, runnerModel(runner, kept))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, runner.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, runnerSettable(runner))...)
}

func (r *RunnerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state RunnerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := runnerInput(plan)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(sendRunnerToken(ctx, req.Config, req.Plan, &req.State, &in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	runner, err := r.client.UpdateRunner(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the runner", err, runnerFields, client.ScopeRunners)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "runner", runner.Warnings, runnerFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, runnerModel(runner, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, runner.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, runnerSettable(runner))...)
}

func (r *RunnerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RunnerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteRunner(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the runner", err, runnerFields, client.ScopeRunners)
	}
}

func (r *RunnerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/build-1")
	if !ok {
		return
	}

	var runner client.Runner
	var err error
	if target.byID() {
		runner, err = r.client.ReadRunner(ctx, target.id)
	} else {
		runner, err = r.client.ImportRunner(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the runner at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, runnerModel(runner, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, runner.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, runnerSettable(runner))...)
}

func runnerSettable(runner client.Runner) any {
	return client.RunnerInput{Name: runner.Name, Labels: runner.Labels}
}

func runnerInput(plan RunnerResourceModel) (client.RunnerInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	return client.RunnerInput{
		Name:   plan.Name.ValueString(),
		Labels: stringsIn(plan.Labels, &diags),
	}, diags
}

func sendRunnerToken(ctx context.Context, config tfsdk.Config, plan tfsdk.Plan, state *tfsdk.State, in *client.RunnerInput) diag.Diagnostics {
	var diags diag.Diagnostics

	if state != nil {
		rewritten, more := writeonly.Token.Rewritten(ctx, plan, *state)
		diags.Append(more...)

		if !rewritten {
			return diags
		}
	}

	token, more := writeonly.Token.Read(ctx, config)
	diags.Append(more...)

	if !token.IsNull() && !token.IsUnknown() {
		in.Token = token.ValueString()
	}

	return diags
}

func runnerCreated(runner client.Runner) diag.Diagnostics {
	var diags diag.Diagnostics

	if runner.IssuedToken {
		diags.AddAttributeWarning(path.Root(writeonly.Token.Name), "Spotlight issued the runner a token Terraform did not keep", runnerIssuedToken)
	}

	return diags
}

func runnerModel(runner client.Runner, kept *RunnerResourceModel) RunnerResourceModel {
	model := RunnerResourceModel{
		ID:           types.StringValue(runner.ID),
		Version:      types.Int64Value(runner.Version),
		Address:      types.StringValue(runner.Address),
		WorkspaceID:  types.StringValue(runner.WorkspaceID),
		Name:         types.StringValue(runner.Name),
		Labels:       stringSet(runner.Labels),
		Token:        types.StringNull(),
		TokenVersion: types.Int64Null(),
		Trust:        textOrNull(runner.Trust),
	}

	if kept != nil {
		model.Name = kept.Name
		model.Labels = kept.Labels
		model.TokenVersion = kept.TokenVersion
	}

	return model
}
