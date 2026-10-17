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
	_ list.ListResource              = &EgressListResource{}
	_ list.ListResourceWithConfigure = &EgressListResource{}
)

// NewEgressListResource returns the spotlight_egress list resource.
func NewEgressListResource() list.ListResource {
	return &EgressListResource{}
}

// EgressListResource lists the egresses in a workspace.
type EgressListResource struct {
	client *client.Client
}

// EgressListResourceModel is the spotlight_egress list block's config.
type EgressListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func (r *EgressListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_egress"
}

func (r *EgressListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the egresses in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
		},
	}
}

func (r *EgressListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *EgressListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config EgressListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	egresses, err := r.client.ListEgresses(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the egresses", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, egress := range limited(egresses, req.Limit) {
			if !push(listResult(ctx, req, egress.ID, egress.Address, egressModel(egress, nil))) {
				return
			}
		}
	}
}
