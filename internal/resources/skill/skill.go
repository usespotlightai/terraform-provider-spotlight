// Package skill is the spotlight_skill resource.
package skill

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const kind = "skill"

type model struct {
	ID          types.String `tfsdk:"id"`
	Version     types.Int64  `tfsdk:"version"`
	Address     types.String `tfsdk:"address"`
	TeamID      types.String `tfsdk:"team_id"`
	Key         types.String `tfsdk:"key"`
	Files       types.Map    `tfsdk:"files"`
	Description types.String `tfsdk:"description"`
	Secrets     types.List   `tfsdk:"secrets"`
	Tools       types.List   `tfsdk:"tools"`
}

var secretType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"name":        types.StringType,
	"description": types.StringType,
}}

// New returns the spotlight_skill resource.
func New() resource.Resource {
	return resourcekit.NewManaged(resourcekit.Spec[spotlight.SkillInput, spotlight.Skill, model]{
		Kind:        kind,
		Schema:      schemaOf,
		AddressedAs: "core/grafana",
		Resources:   func(api spotlight.API) spotlight.Resources[spotlight.SkillInput, spotlight.Skill] { return api.Skills },
		Owner:       func(m model) string { return m.TeamID.ValueString() },
		Input:       inputOf,
		Stored:      stored,
		Meta:        func(s spotlight.Skill) spotlight.Meta { return s.Meta },
		Settable:    func(s spotlight.Skill) any { return spotlight.SkillInput{Key: s.Key, Files: s.Files} },
		Identity:    func(m model) (string, int64) { return m.ID.ValueString(), m.Version.ValueInt64() },
	})
}

var schemaOf = schema.Schema{
	MarkdownDescription: "A skill a team's stations can give their agents: a folder of files read from `SKILL.md`. Import it as `<workspace id>/<team key>/<key>`.",
	Attributes: map[string]schema.Attribute{
		"id":      resourcekit.IDAttribute(kind),
		"version": resourcekit.VersionAttribute(kind),
		"address": resourcekit.AddressAttribute("core/grafana"),
		"team_id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The team the skill belongs to. Changing it replaces the skill.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"key": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "What stations give the skill by. It is fixed for the skill's life, so changing it replaces the skill.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"files": schema.MapAttribute{
			Required:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Each file in the skill's folder, by its path within it, usually read with `file()`. `SKILL.md` is the one the skill is read from.",
		},
		"description": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "What `SKILL.md` says the skill is for.",
		},
		"secrets": schema.ListNestedAttribute{
			Computed:            true,
			MarkdownDescription: "The secrets `SKILL.md` asks for.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"name":        schema.StringAttribute{Computed: true, MarkdownDescription: "The secret's name."},
					"description": schema.StringAttribute{Computed: true, MarkdownDescription: "What the secret is for."},
				},
			},
		},
		"tools": schema.ListAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "The tools `SKILL.md` uses.",
		},
	},
}

func inputOf(plan model) (spotlight.SkillInput, diag.Diagnostics) {
	files := map[string]string{}
	diags := plan.Files.ElementsAs(context.Background(), &files, false)

	return spotlight.SkillInput{Key: plan.Key.ValueString(), Files: files}, diags
}

func stored(answer spotlight.Skill, kept *model) (model, diag.Diagnostics) {
	var diags diag.Diagnostics

	files, more := types.MapValueFrom(context.Background(), types.StringType, answer.Files)
	diags.Append(more...)

	secrets := make([]attr.Value, 0, len(answer.Secrets))
	for _, each := range answer.Secrets {
		secrets = append(secrets, types.ObjectValueMust(secretType.AttrTypes, map[string]attr.Value{
			"name":        types.StringValue(each.Name),
			"description": types.StringValue(each.Description),
		}))
	}

	tools, more := types.ListValueFrom(context.Background(), types.StringType, orEmpty(answer.Tools))
	diags.Append(more...)

	held := model{
		ID:          types.StringValue(answer.ID),
		Version:     types.Int64Value(answer.Version),
		Address:     types.StringValue(answer.Address),
		TeamID:      types.StringValue(answer.TeamID),
		Key:         types.StringValue(answer.Key),
		Files:       files,
		Description: types.StringValue(answer.Description),
		Secrets:     types.ListValueMust(secretType, secrets),
		Tools:       tools,
	}

	if kept != nil {
		held.Key = kept.Key
		held.Files = kept.Files
	}

	return held, diags
}

func orEmpty(held []string) []string {
	if held == nil {
		return []string{}
	}

	return held
}
