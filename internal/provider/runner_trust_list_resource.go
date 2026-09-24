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
	_ list.ListResource              = &RunnerTrustListResource{}
	_ list.ListResourceWithConfigure = &RunnerTrustListResource{}
)

// NewRunnerTrustListResource returns the spotlight_runner_trust list resource.
func NewRunnerTrustListResource() list.ListResource {
	return &RunnerTrustListResource{}
}

// RunnerTrustListResource lists the runner trusts in a workspace.
type RunnerTrustListResource struct {
	client *client.Client
}

// RunnerTrustListResourceModel is the spotlight_runner_trust list block's config.
type RunnerTrustListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func (r *RunnerTrustListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_runner_trust"
}

func (r *RunnerTrustListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the runner trusts in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
		},
	}
}

func (r *RunnerTrustListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *RunnerTrustListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config RunnerTrustListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	runnerTrusts, err := r.client.ListRunnerTrusts(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the runner trusts", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, runnerTrust := range limited(runnerTrusts, req.Limit) {
			if !push(listResult(ctx, req, runnerTrust.ID, runnerTrust.Address, runnerTrustModel(runnerTrust, nil))) {
				return
			}
		}
	}
}
