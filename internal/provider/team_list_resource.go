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
	_ list.ListResource              = &TeamListResource{}
	_ list.ListResourceWithConfigure = &TeamListResource{}
)

// NewTeamListResource returns the spotlight_team list resource.
func NewTeamListResource() list.ListResource {
	return &TeamListResource{}
}

// TeamListResource lists the teams in a workspace.
type TeamListResource struct {
	client *client.Client
}

// TeamListResourceModel is the spotlight_team list block's config.
type TeamListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func (r *TeamListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

func (r *TeamListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the teams in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
		},
	}
}

func (r *TeamListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *TeamListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config TeamListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	teams, err := r.client.ListTeams(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the teams", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, team := range limited(teams, req.Limit) {
			if !push(listResult(ctx, req, team.ID, team.Address, teamModel(team, nil))) {
				return
			}
		}
	}
}
