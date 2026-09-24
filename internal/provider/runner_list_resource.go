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
	_ list.ListResource              = &RunnerListResource{}
	_ list.ListResourceWithConfigure = &RunnerListResource{}
)

// NewRunnerListResource returns the spotlight_runner list resource.
func NewRunnerListResource() list.ListResource {
	return &RunnerListResource{}
}

// RunnerListResource lists the runners in a workspace.
type RunnerListResource struct {
	client *client.Client
}

// RunnerListResourceModel is the spotlight_runner list block's config.
type RunnerListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func (r *RunnerListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_runner"
}

func (r *RunnerListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the runners in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
		},
	}
}

func (r *RunnerListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *RunnerListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config RunnerListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	runners, err := r.client.ListRunners(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the runners", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, runner := range limited(runners, req.Limit) {
			if !push(listResult(ctx, req, runner.ID, runner.Address, runnerModel(runner, nil))) {
				return
			}
		}
	}
}
