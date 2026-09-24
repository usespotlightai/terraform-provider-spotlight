package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

var (
	_ resource.Resource                = &WorkspaceRunnersResource{}
	_ resource.ResourceWithConfigure   = &WorkspaceRunnersResource{}
	_ resource.ResourceWithImportState = &WorkspaceRunnersResource{}
	_ resource.ResourceWithIdentity    = &WorkspaceRunnersResource{}
)

// NewWorkspaceRunnersResource returns the spotlight_workspace_runners resource.
func NewWorkspaceRunnersResource() resource.Resource {
	return &WorkspaceRunnersResource{}
}

// WorkspaceRunnersResource is the spotlight_workspace_runners resource.
type WorkspaceRunnersResource struct {
	client *client.Client
}

// WorkspaceRunnersResourceModel is a spotlight_workspace_runners as Terraform holds it.
type WorkspaceRunnersResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Version     types.Int64  `tfsdk:"version"`
	Address     types.String `tfsdk:"address"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
	Runners     types.Set    `tfsdk:"runners"`
	Trusts      types.Set    `tfsdk:"trusts"`
	Drift       types.Set    `tfsdk:"drift"`
}

var workspaceRunnersFields = fieldpath.Locator{Schema: workspaceRunnersSchema().Type()}

func (r *WorkspaceRunnersResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workspace_runners"
}

func (r *WorkspaceRunnersResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = workspaceRunnersSchema()
}

func workspaceRunnersSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "The runners a workspace means to have, declared once for the workspace. A runner enrolled all the same is drift: " +
			"it still runs, and every plan warns about it until it is declared here or deleted. So does a name here that no runner or trust answers to. " +
			"Writing the declaration needs a token granted the `runners` scope. Import it as `<workspace id>/<workspace slug>`.",
		Attributes: map[string]schema.Attribute{
			"id":           idAttribute("declaration"),
			"version":      versionAttribute("declaration"),
			"address":      addressAttribute("acme"),
			"workspace_id": ownerAttribute("workspace", "declaration"),
			"runners":      stringSetAttribute("The runners the workspace means to have, by name."),
			"trusts":       stringSetAttribute("The runner trusts, by key, whose runners the workspace means to have."),
			"drift": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "The runners enrolled all the same, outside what is declared.",
			},
		},
	}
}

func (r *WorkspaceRunnersResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("workspace runners")
}

func (r *WorkspaceRunnersResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *WorkspaceRunnersResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan WorkspaceRunnersResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := workspaceRunnersInput(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	workspaceRunners, err := r.client.CreateWorkspaceRunners(ctx, plan.WorkspaceID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the workspace runners", err, workspaceRunnersFields, client.ScopeRunners)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "workspace runners", workspaceRunners.Warnings, workspaceRunnersFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceRunnersModel(workspaceRunners, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, workspaceRunners.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, workspaceRunnersSettable(workspaceRunners))...)
}

func (r *WorkspaceRunnersResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state WorkspaceRunnersResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	workspaceRunners, err := r.client.ReadWorkspaceRunners(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the workspace runners", err, workspaceRunnersFields, client.ScopeRead)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "workspace runners", workspaceRunners.Warnings, workspaceRunnersFields)

	kept := &state
	if !answerUnchanged(ctx, req.Private, workspaceRunnersSettable(workspaceRunners)) {
		kept = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceRunnersModel(workspaceRunners, kept))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, workspaceRunners.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, workspaceRunnersSettable(workspaceRunners))...)
}

func (r *WorkspaceRunnersResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state WorkspaceRunnersResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := workspaceRunnersInput(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	workspaceRunners, err := r.client.UpdateWorkspaceRunners(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the workspace runners", err, workspaceRunnersFields, client.ScopeRunners)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "workspace runners", workspaceRunners.Warnings, workspaceRunnersFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceRunnersModel(workspaceRunners, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, workspaceRunners.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, workspaceRunnersSettable(workspaceRunners))...)
}

func (r *WorkspaceRunnersResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state WorkspaceRunnersResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteWorkspaceRunners(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the workspace runners", err, workspaceRunnersFields, client.ScopeRunners)
	}
}

func (r *WorkspaceRunnersResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/acme")
	if !ok {
		return
	}

	var workspaceRunners client.WorkspaceRunners
	var err error
	if target.byID() {
		workspaceRunners, err = r.client.ReadWorkspaceRunners(ctx, target.id)
	} else {
		workspaceRunners, err = r.client.ImportWorkspaceRunners(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the workspace runners at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceRunnersModel(workspaceRunners, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, workspaceRunners.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, workspaceRunnersSettable(workspaceRunners))...)
}

func workspaceRunnersSettable(declared client.WorkspaceRunners) any {
	return declared.WorkspaceRunnersInput
}

func workspaceRunnersInput(plan WorkspaceRunnersResourceModel) (client.WorkspaceRunnersInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	return client.WorkspaceRunnersInput{
		Runners: stringsIn(plan.Runners, &diags),
		Trusts:  stringsIn(plan.Trusts, &diags),
	}, diags
}

func workspaceRunnersModel(declared client.WorkspaceRunners, kept *WorkspaceRunnersResourceModel) WorkspaceRunnersResourceModel {
	model := WorkspaceRunnersResourceModel{
		ID:          types.StringValue(declared.ID),
		Version:     types.Int64Value(declared.Version),
		Address:     types.StringValue(declared.Address),
		WorkspaceID: types.StringValue(declared.WorkspaceID),
		Runners:     stringSet(declared.Runners),
		Trusts:      stringSet(declared.Trusts),
		Drift:       stringSet(declared.Drift),
	}

	if kept != nil {
		model.Runners = kept.Runners
		model.Trusts = kept.Trusts
	}

	return model
}
