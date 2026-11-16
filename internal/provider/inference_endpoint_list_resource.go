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
	_ list.ListResource              = &InferenceEndpointListResource{}
	_ list.ListResourceWithConfigure = &InferenceEndpointListResource{}
)

func NewInferenceEndpointListResource() list.ListResource {
	return &InferenceEndpointListResource{}
}

type InferenceEndpointListResource struct {
	client *client.Client
}

type InferenceEndpointListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func (r *InferenceEndpointListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_inference_endpoint"
}

func (r *InferenceEndpointListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the inference endpoints in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
		},
	}
}

func (r *InferenceEndpointListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *InferenceEndpointListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config InferenceEndpointListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	endpoints, err := r.client.ListInferenceEndpoints(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the inference endpoints", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, endpoint := range limited(endpoints, req.Limit) {
			if !push(listResult(ctx, req, endpoint.ID, endpoint.Address, inferenceEndpointModel(endpoint, nil))) {
				return
			}
		}
	}
}
