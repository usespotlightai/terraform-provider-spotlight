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
	_ list.ListResource              = &HookListResource{}
	_ list.ListResourceWithConfigure = &HookListResource{}
)

// NewHookListResource returns the spotlight_hook list resource.
func NewHookListResource() list.ListResource {
	return &HookListResource{}
}

// HookListResource lists the hooks in a workspace.
type HookListResource struct {
	client *client.Client
}

// HookListResourceModel is the spotlight_hook list block's config.
type HookListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
	TeamID      types.String `tfsdk:"team_id"`
}

func (r *HookListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hook"
}

func (r *HookListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the hooks in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
			"team_id":      teamIDListAttribute(),
		},
	}
}

func (r *HookListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *HookListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config HookListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	hooks, err := r.client.ListHooks(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the hooks", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, hook := range limited(ofTeam(hooks, config.TeamID.ValueString(), func(each client.Hook) string { return each.TeamID }), req.Limit) {
			if !push(listResult(ctx, req, hook.ID, hook.Address, hookModel(hook, nil))) {
				return
			}
		}
	}
}
