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
	_ list.ListResource              = &SkillListResource{}
	_ list.ListResourceWithConfigure = &SkillListResource{}
)

// NewSkillListResource returns the spotlight_skill list resource.
func NewSkillListResource() list.ListResource {
	return &SkillListResource{}
}

// SkillListResource lists the skills in a workspace.
type SkillListResource struct {
	client *client.Client
}

// SkillListResourceModel is the spotlight_skill list block's config.
type SkillListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
	TeamID      types.String `tfsdk:"team_id"`
}

func (r *SkillListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_skill"
}

func (r *SkillListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the skills in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
			"team_id":      teamIDListAttribute(),
		},
	}
}

func (r *SkillListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *SkillListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config SkillListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	skills, err := r.client.ListSkills(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the skills", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, skill := range limited(heldBy(skills, config.TeamID.ValueString(), func(each client.Skill) string { return each.TeamID }), req.Limit) {
			if !push(listResult(ctx, req, skill.ID, skill.Address, skillModel(skill, nil))) {
				return
			}
		}
	}
}
