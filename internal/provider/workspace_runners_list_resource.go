package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
)

var (
	_ list.ListResource              = &WorkspaceRunnersListResource{}
	_ list.ListResourceWithConfigure = &WorkspaceRunnersListResource{}
)

// NewWorkspaceRunnersListResource returns the spotlight_workspace_runners list resource.
func NewWorkspaceRunnersListResource() list.ListResource {
	return &WorkspaceRunnersListResource{}
}

// WorkspaceRunnersListResource lists the runner declarations in a workspace.
type WorkspaceRunnersListResource struct {
	client *client.Client
}

// WorkspaceRunnersListResourceModel is the spotlight_workspace_runners list block's config.
type WorkspaceRunnersListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func (r *WorkspaceRunnersListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workspace_runners"
}

func (r *WorkspaceRunnersListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the runner declarations in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
		},
	}
}

func (r *WorkspaceRunnersListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *WorkspaceRunnersListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config WorkspaceRunnersListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	declarations, err := r.client.ListWorkspaceRunners(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the runner declarations", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, declared := range limited(declarations, req.Limit) {
			if !push(listResult(ctx, req, declared.ID, declared.Address, workspaceRunnersModel(declared, nil))) {
				return
			}
		}
	}
}
