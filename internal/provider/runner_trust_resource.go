package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

var (
	_ resource.Resource                = &RunnerTrustResource{}
	_ resource.ResourceWithConfigure   = &RunnerTrustResource{}
	_ resource.ResourceWithImportState = &RunnerTrustResource{}
	_ resource.ResourceWithIdentity    = &RunnerTrustResource{}
)

// NewRunnerTrustResource returns the spotlight_runner_trust resource.
func NewRunnerTrustResource() resource.Resource {
	return &RunnerTrustResource{}
}

// RunnerTrustResource is the spotlight_runner_trust resource.
type RunnerTrustResource struct {
	client *client.Client
}

// RunnerTrustResourceModel is a spotlight_runner_trust as Terraform holds it.
type RunnerTrustResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Version     types.Int64  `tfsdk:"version"`
	Address     types.String `tfsdk:"address"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
	Key         types.String `tfsdk:"key"`
	Issuer      types.String `tfsdk:"issuer"`
	Subject     types.String `tfsdk:"subject"`
	Audience    types.String `tfsdk:"audience"`
	Labels      types.Set    `tfsdk:"labels"`
}

var runnerTrustFields = fieldpath.Locator{Schema: runnerTrustSchema().Type()}

func (r *RunnerTrustResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_runner_trust"
}

func (r *RunnerTrustResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = runnerTrustSchema()
}

func runnerTrustSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "A workload whose platform-issued tokens let a `spotlight-host` claim work, as a Kubernetes service account's do: a host there holds no Spotlight token " +
			"and claims through the trust with its workload token, given `SPOTLIGHT_TRUST=<trust id>` and `SPOTLIGHT_OIDC_TOKEN_FILE=<projected token path>`. " +
			"Writing runner trusts needs a token granted the `runners` scope. Import it as `<workspace id>/<key>`.",
		Attributes: map[string]schema.Attribute{
			"id":           idAttribute("runner trust"),
			"version":      versionAttribute("runner trust"),
			"address":      addressAttribute("production-cluster"),
			"workspace_id": ownerAttribute("workspace", "runner trust"),
			"key":          fixedAttribute("Names the trust, as in `production-cluster`.", "runner trust"),
			"issuer": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The https URL the platform signs its workloads' tokens as; its keys are found through OIDC discovery.",
			},
			"subject": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The workload a token must be issued to, exactly, as in `system:serviceaccount:spotlight:runner`.",
			},
			"audience": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("spotlight"),
				MarkdownDescription: "The audience a token must be issued for.",
			},
			"labels": stringSetAttribute("What a line or station may name in `runners` to place its work on the runners claimed through here, as in `gpu`."),
		},
	}
}

func (r *RunnerTrustResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("runner trust")
}

func (r *RunnerTrustResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *RunnerTrustResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RunnerTrustResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := runnerTrustInput(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	runnerTrust, err := r.client.CreateRunnerTrust(ctx, plan.WorkspaceID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the runner trust", err, runnerTrustFields, client.ScopeRunners)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "runner trust", runnerTrust.Warnings, runnerTrustFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, runnerTrustModel(runnerTrust, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, runnerTrust.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, runnerTrustSettable(runnerTrust))...)
}

func (r *RunnerTrustResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RunnerTrustResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	runnerTrust, err := r.client.ReadRunnerTrust(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the runner trust", err, runnerTrustFields, client.ScopeRead)
		return
	}

	kept := &state
	if !answerUnchanged(ctx, req.Private, runnerTrustSettable(runnerTrust)) {
		kept = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, runnerTrustModel(runnerTrust, kept))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, runnerTrust.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, runnerTrustSettable(runnerTrust))...)
}

func (r *RunnerTrustResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state RunnerTrustResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := runnerTrustInput(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	runnerTrust, err := r.client.UpdateRunnerTrust(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the runner trust", err, runnerTrustFields, client.ScopeRunners)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "runner trust", runnerTrust.Warnings, runnerTrustFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, runnerTrustModel(runnerTrust, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, runnerTrust.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, runnerTrustSettable(runnerTrust))...)
}

func (r *RunnerTrustResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RunnerTrustResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteRunnerTrust(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the runner trust", err, runnerTrustFields, client.ScopeRunners)
	}
}

func (r *RunnerTrustResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/production-cluster")
	if !ok {
		return
	}

	var runnerTrust client.RunnerTrust
	var err error
	if target.byID() {
		runnerTrust, err = r.client.ReadRunnerTrust(ctx, target.id)
	} else {
		runnerTrust, err = r.client.ImportRunnerTrust(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the runner trust at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, runnerTrustModel(runnerTrust, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, runnerTrust.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, runnerTrustSettable(runnerTrust))...)
}

func runnerTrustSettable(trust client.RunnerTrust) any {
	return trust.RunnerTrustInput
}

func runnerTrustInput(plan RunnerTrustResourceModel) (client.RunnerTrustInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	return client.RunnerTrustInput{
		Key:      plan.Key.ValueString(),
		Issuer:   plan.Issuer.ValueString(),
		Subject:  plan.Subject.ValueString(),
		Audience: plan.Audience.ValueString(),
		Labels:   stringsIn(plan.Labels, &diags),
	}, diags
}

func runnerTrustModel(trust client.RunnerTrust, kept *RunnerTrustResourceModel) RunnerTrustResourceModel {
	model := RunnerTrustResourceModel{
		ID:          types.StringValue(trust.ID),
		Version:     types.Int64Value(trust.Version),
		Address:     types.StringValue(trust.Address),
		WorkspaceID: types.StringValue(trust.WorkspaceID),
		Key:         types.StringValue(trust.Key),
		Issuer:      types.StringValue(trust.Issuer),
		Subject:     types.StringValue(trust.Subject),
		Audience:    types.StringValue(trust.Audience),
		Labels:      stringSet(trust.Labels),
	}

	if kept != nil {
		model.Key = kept.Key
		model.Issuer = kept.Issuer
		model.Subject = kept.Subject
		model.Audience = kept.Audience
		model.Labels = kept.Labels
	}

	return model
}
