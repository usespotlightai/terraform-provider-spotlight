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
	_ list.ListResource              = &WorkspaceTokenTrustListResource{}
	_ list.ListResourceWithConfigure = &WorkspaceTokenTrustListResource{}
)

// NewWorkspaceTokenTrustListResource returns the
// spotlight_workspace_token_trust list resource.
func NewWorkspaceTokenTrustListResource() list.ListResource {
	return &WorkspaceTokenTrustListResource{}
}

// WorkspaceTokenTrustListResource lists the workspace token trusts in a
// workspace.
type WorkspaceTokenTrustListResource struct {
	client *client.Client
}

// WorkspaceTokenTrustListResourceModel is the
// spotlight_workspace_token_trust list block's config.
type WorkspaceTokenTrustListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func (r *WorkspaceTokenTrustListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workspace_token_trust"
}

func (r *WorkspaceTokenTrustListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the workspace token trusts in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
		},
	}
}

func (r *WorkspaceTokenTrustListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *WorkspaceTokenTrustListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config WorkspaceTokenTrustListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	trusts, err := r.client.ListWorkspaceTokenTrusts(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the workspace token trusts", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, trust := range limited(trusts, req.Limit) {
			if !push(listResult(ctx, req, trust.ID, trust.Address, workspaceTokenTrustModel(trust, nil))) {
				return
			}
		}
	}
}
