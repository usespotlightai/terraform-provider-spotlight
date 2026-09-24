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
	_ list.ListResource              = &HarnessProfileListResource{}
	_ list.ListResourceWithConfigure = &HarnessProfileListResource{}
)

// NewHarnessProfileListResource returns the spotlight_harness_profile list resource.
func NewHarnessProfileListResource() list.ListResource {
	return &HarnessProfileListResource{}
}

// HarnessProfileListResource lists the harness profiles in a workspace.
type HarnessProfileListResource struct {
	client *client.Client
}

// HarnessProfileListResourceModel is the spotlight_harness_profile list block's config.
type HarnessProfileListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
	TeamID      types.String `tfsdk:"team_id"`
}

func (r *HarnessProfileListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_harness_profile"
}

func (r *HarnessProfileListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the harness profiles in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
			"team_id":      teamIDListAttribute(),
		},
	}
}

func (r *HarnessProfileListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *HarnessProfileListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config HarnessProfileListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	harnessProfiles, err := r.client.ListHarnessProfiles(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the harness profiles", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, harnessProfile := range limited(heldBy(harnessProfiles, config.TeamID.ValueString(), func(each client.HarnessProfile) string { return each.TeamID }), req.Limit) {
			if !push(listResult(ctx, req, harnessProfile.ID, harnessProfile.Address, harnessProfileModel(harnessProfile, nil))) {
				return
			}
		}
	}
}
