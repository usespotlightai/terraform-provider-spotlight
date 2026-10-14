package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
)

var (
	_ list.ListResource              = &WorkspaceListResource{}
	_ list.ListResourceWithConfigure = &WorkspaceListResource{}
)

// NewWorkspaceListResource returns the spotlight_workspace list resource.
func NewWorkspaceListResource() list.ListResource {
	return &WorkspaceListResource{}
}

// WorkspaceListResource lists the workspaces that the caller can read. For an
// API token, this is the workspace of the token.
type WorkspaceListResource struct {
	client *client.Client
}

func (r *WorkspaceListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workspace"
}

func (r *WorkspaceListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the workspaces the caller may read; an API token reads only its own.",
	}
}

func (r *WorkspaceListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *WorkspaceListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	workspaces, err := r.client.ListWorkspaces(ctx)
	if err != nil {
		stream.Results = listFailed("Could not list the workspaces", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, workspace := range limited(workspaces, req.Limit) {
			if !push(listResult(ctx, req, workspace.ID, workspace.Address, workspaceModel(workspace, nil))) {
				return
			}
		}
	}
}
