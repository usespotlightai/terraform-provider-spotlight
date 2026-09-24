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
	_ list.ListResource              = &LineListResource{}
	_ list.ListResourceWithConfigure = &LineListResource{}
)

// NewLineListResource returns the spotlight_line list resource.
func NewLineListResource() list.ListResource {
	return &LineListResource{}
}

// LineListResource lists the lines in a workspace.
type LineListResource struct {
	client *client.Client
}

// LineListResourceModel is the spotlight_line list block's config.
type LineListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
	TeamID      types.String `tfsdk:"team_id"`
}

func (r *LineListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_line"
}

func (r *LineListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the lines in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
			"team_id":      teamIDListAttribute(),
		},
	}
}

func (r *LineListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *LineListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config LineListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	lines, err := r.client.ListLines(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the lines", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, line := range limited(heldBy(lines, config.TeamID.ValueString(), func(each client.Line) string { return each.TeamID }), req.Limit) {
			if !push(listResult(ctx, req, line.ID, line.Address, lineModel(line, nil))) {
				return
			}
		}
	}
}
