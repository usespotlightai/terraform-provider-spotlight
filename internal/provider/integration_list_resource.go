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
	_ list.ListResource              = &IntegrationListResource{}
	_ list.ListResourceWithConfigure = &IntegrationListResource{}
)

// NewIntegrationListResource returns the spotlight_integration list resource.
func NewIntegrationListResource() list.ListResource {
	return &IntegrationListResource{}
}

// IntegrationListResource lists the integrations in a workspace.
type IntegrationListResource struct {
	client *client.Client
}

// IntegrationListResourceModel is the spotlight_integration list block's config.
type IntegrationListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
	TeamID      types.String `tfsdk:"team_id"`
}

func (r *IntegrationListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integration"
}

func (r *IntegrationListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the integrations in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
			"team_id":      teamIDListAttribute(),
		},
	}
}

func (r *IntegrationListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *IntegrationListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config IntegrationListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	integrations, err := r.client.ListIntegrations(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the integrations", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, integration := range limited(ofTeam(integrations, config.TeamID.ValueString(), func(each client.Integration) string { return each.TeamID }), req.Limit) {
			if !push(listResult(ctx, req, integration.ID, integration.Address, integrationModel(integration, nil))) {
				return
			}
		}
	}
}
