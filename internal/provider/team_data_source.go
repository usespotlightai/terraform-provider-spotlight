package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

var (
	_ datasource.DataSource              = &TeamDataSource{}
	_ datasource.DataSourceWithConfigure = &TeamDataSource{}
)

// NewTeamDataSource returns the spotlight_team data source.
func NewTeamDataSource() datasource.DataSource {
	return &TeamDataSource{}
}

// TeamDataSource reads a team by its key, without managing it.
type TeamDataSource struct {
	client *client.Client
}

// TeamDataSourceModel is the spotlight_team data source.
type TeamDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Address     types.String `tfsdk:"address"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
	Key         types.String `tfsdk:"key"`
	Name        types.String `tfsdk:"name"`
}

func (d *TeamDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

func (d *TeamDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A team in a Spotlight workspace, read by its key, for configuration that uses a team another configuration manages.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The id Spotlight issued the team.",
			},
			"address": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Names the team within its workspace, as in `core`.",
			},
			"workspace_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The workspace the team belongs to.",
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The team's key, as in `core`.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "What the team is called.",
			},
		},
	}
}

func (d *TeamDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (d *TeamDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TeamDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	team, err := d.client.ImportTeam(ctx, config.WorkspaceID.ValueString(), config.Key.ValueString())
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the team "+config.Key.ValueString(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, TeamDataSourceModel{
		ID:          types.StringValue(team.ID),
		Address:     types.StringValue(team.Address),
		WorkspaceID: types.StringValue(team.WorkspaceID),
		Key:         types.StringValue(team.Key),
		Name:        types.StringValue(team.Name),
	})...)
}
