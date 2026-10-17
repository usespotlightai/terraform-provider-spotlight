package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

var (
	_ resource.Resource                = &WorkspaceResource{}
	_ resource.ResourceWithConfigure   = &WorkspaceResource{}
	_ resource.ResourceWithImportState = &WorkspaceResource{}
	_ resource.ResourceWithIdentity    = &WorkspaceResource{}
)

const workspaceForbiddenToCreate = "An API token acts only inside its own workspace, so it may not create a new one. " +
	"A person creates a workspace by signing up in the web app; import it rather than creating it, with an import block that names its slug:\n\n" +
	"  import {\n    to = spotlight_workspace.<name>\n    id = \"<slug>\"\n  }"

// NewWorkspaceResource returns the spotlight_workspace resource.
func NewWorkspaceResource() resource.Resource {
	return &WorkspaceResource{}
}

// WorkspaceResource is the spotlight_workspace resource.
type WorkspaceResource struct {
	client *client.Client
}

// WorkspaceResourceModel is a spotlight_workspace as Terraform stores it.
type WorkspaceResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Version types.Int64  `tfsdk:"version"`
	Address types.String `tfsdk:"address"`
	Name    types.String `tfsdk:"name"`
	Slug    types.String `tfsdk:"slug"`
}

var workspaceFields = fieldpath.Locator{Schema: workspaceSchema().Type()}

func (r *WorkspaceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workspace"
}

func (r *WorkspaceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = workspaceSchema()
}

func workspaceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "A Spotlight workspace. A person creates it by signing up in the web app; Terraform adopts it with an `import` block that names its slug, then manages everything inside it.\n\n" +
			"Creating a workspace from Terraform needs a caller allowed to open workspaces, and API tokens are not: a token acts only inside its own workspace. Import the workspace rather than creating it.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute("workspace"),
			"version": versionAttribute("workspace"),
			"address": addressAttribute("acme"),
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "What the workspace is called.",
			},
			"slug": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Where the workspace lives in the web app. It is fixed for the workspace's life, so changing it replaces the workspace.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *WorkspaceResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("workspace")
}

func (r *WorkspaceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *WorkspaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan WorkspaceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	workspace, err := r.client.CreateWorkspace(ctx, workspaceInput(plan))
	if client.IsForbidden(err) {
		resp.Diagnostics.AddError("Could not create the workspace", workspaceForbiddenToCreate)
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the workspace", err, workspaceFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "workspace", workspace.Warnings, workspaceFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceModel(workspace, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, workspace.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, workspaceSettable(workspace))...)
}

func (r *WorkspaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state WorkspaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	workspace, err := r.client.ReadWorkspace(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the workspace", err, workspaceFields, client.ScopeRead)
		return
	}

	kept := &state
	if !answerUnchanged(ctx, req.Private, workspaceSettable(workspace)) {
		kept = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceModel(workspace, kept))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, workspace.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, workspaceSettable(workspace))...)
}

func (r *WorkspaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state WorkspaceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	workspace, err := r.client.UpdateWorkspace(ctx, state.ID.ValueString(), state.Version.ValueInt64(), workspaceInput(plan))
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the workspace", err, workspaceFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "workspace", workspace.Warnings, workspaceFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceModel(workspace, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, workspace.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, workspaceSettable(workspace))...)
}

func (r *WorkspaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state WorkspaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteWorkspace(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the workspace", err, workspaceFields, client.ScopeConfigure)
	}
}

func (r *WorkspaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := slugImportTargetOf(ctx, req, &resp.Diagnostics)
	if !ok {
		return
	}

	var workspace client.Workspace
	var err error
	if target.byID() {
		workspace, err = r.client.ReadWorkspace(ctx, target.id)
	} else {
		workspace, err = r.client.ImportWorkspace(ctx, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the workspace at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceModel(workspace, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, workspace.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, workspaceSettable(workspace))...)
}

func workspaceInput(plan WorkspaceResourceModel) client.WorkspaceInput {
	return client.WorkspaceInput{Name: plan.Name.ValueString(), Slug: plan.Slug.ValueString()}
}

func workspaceSettable(workspace client.Workspace) any {
	return client.WorkspaceInput{Name: workspace.Name, Slug: workspace.Slug}
}

func workspaceModel(workspace client.Workspace, kept *WorkspaceResourceModel) WorkspaceResourceModel {
	model := WorkspaceResourceModel{
		ID:      types.StringValue(workspace.ID),
		Version: types.Int64Value(workspace.Version),
		Address: types.StringValue(workspace.Address),
		Name:    types.StringValue(workspace.Name),
		Slug:    types.StringValue(workspace.Slug),
	}

	if kept != nil {
		model.Name = kept.Name
		model.Slug = kept.Slug
	}

	return model
}
