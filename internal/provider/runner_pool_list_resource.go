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
	_ list.ListResource              = &RunnerPoolListResource{}
	_ list.ListResourceWithConfigure = &RunnerPoolListResource{}
)

// NewRunnerPoolListResource returns the spotlight_runner_pool list resource.
func NewRunnerPoolListResource() list.ListResource {
	return &RunnerPoolListResource{}
}

// RunnerPoolListResource lists the runner pools in a workspace.
type RunnerPoolListResource struct {
	client *client.Client
}

// RunnerPoolListResourceModel is the spotlight_runner_pool list block's config.
type RunnerPoolListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func (r *RunnerPoolListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_runner_pool"
}

func (r *RunnerPoolListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the runner pools in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
		},
	}
}

func (r *RunnerPoolListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *RunnerPoolListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config RunnerPoolListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	pools, err := r.client.ListRunnerPools(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the runner pools", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, pool := range limited(pools, req.Limit) {
			if !push(listResult(ctx, req, pool.ID, pool.Address, runnerPoolModel(pool, nil))) {
				return
			}
		}
	}
}
