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
	_ datasource.DataSource              = &WorkspaceDataSource{}
	_ datasource.DataSourceWithConfigure = &WorkspaceDataSource{}
)

// NewWorkspaceDataSource returns the spotlight_workspace data source.
func NewWorkspaceDataSource() datasource.DataSource {
	return &WorkspaceDataSource{}
}

// WorkspaceDataSource reads a workspace by its slug, without managing it.
type WorkspaceDataSource struct {
	client *client.Client
}

// WorkspaceDataSourceModel is the spotlight_workspace data source.
type WorkspaceDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	Address types.String `tfsdk:"address"`
	Name    types.String `tfsdk:"name"`
	Slug    types.String `tfsdk:"slug"`
}

func (d *WorkspaceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workspace"
}

func (d *WorkspaceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Spotlight workspace, read by its slug, for configuration that works inside a workspace without managing the workspace itself.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The id Spotlight issued the workspace.",
			},
			"address": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Names the workspace, as in `acme`.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "What the workspace is called.",
			},
			"slug": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Where the workspace lives in the web app, as in `acme`.",
			},
		},
	}
}

func (d *WorkspaceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (d *WorkspaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config WorkspaceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	workspace, err := d.client.ImportWorkspace(ctx, config.Slug.ValueString())
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the workspace "+config.Slug.ValueString(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, WorkspaceDataSourceModel{
		ID:      types.StringValue(workspace.ID),
		Address: types.StringValue(workspace.Address),
		Name:    types.StringValue(workspace.Name),
		Slug:    types.StringValue(workspace.Slug),
	})...)
}
