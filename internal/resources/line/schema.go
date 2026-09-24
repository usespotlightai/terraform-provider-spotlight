package line

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
)

var (
	noStrings    = types.ListValueMust(types.StringType, []attr.Value{})
	noStringSet  = types.SetValueMust(types.StringType, []attr.Value{})
	fixedForLife = []planmodifier.String{stringplanmodifier.RequiresReplace()}
)

func optional(description string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, MarkdownDescription: description}
}

func stringList(description string) schema.ListAttribute {
	return schema.ListAttribute{
		Optional:            true,
		Computed:            true,
		ElementType:         types.StringType,
		Default:             listdefault.StaticValue(noStrings),
		MarkdownDescription: description,
	}
}

func flag(description string, fallback bool) schema.BoolAttribute {
	return schema.BoolAttribute{
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(fallback),
		MarkdownDescription: description,
	}
}

func derived(description string) schema.BoolAttribute {
	return schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: description}
}

var schemaOf = schema.Schema{
	MarkdownDescription: "A line: the stations a team's tasks move along, in order, and how they move. Import it as `<workspace id>/<team key>/<key>`.",
	Attributes: map[string]schema.Attribute{
		"id":      resourcekit.IDAttribute(kind),
		"version": resourcekit.VersionAttribute(kind),
		"address": resourcekit.AddressAttribute("core/delivery"),
		"team_id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The team the line belongs to. Changing it replaces the line.",
			PlanModifiers:       fixedForLife,
		},
		"key": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Names the line within its team. It is fixed for the line's life, so changing it replaces the line.",
			PlanModifiers:       fixedForLife,
		},
		"name": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "What the line is called.",
		},
		"role": optional("The role the line plays for its team, as in `delivery` or `review`. A role names one line."),
		"repositories": schema.SetAttribute{
			Optional:            true,
			Computed:            true,
			ElementType:         types.StringType,
			Default:             setdefault.StaticValue(noStringSet),
			MarkdownDescription: "The repositories the line works on, each named `owner/name`.",
		},
	},
	Blocks: map[string]schema.Block{
		"station": schema.ListNestedBlock{
			MarkdownDescription: "A station along the line, in order. A line has at least one.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"key": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Names the station within the line.",
					},
					"name": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "What the station is called on the board.",
					},
					"kind": schema.StringAttribute{
						Optional:            true,
						Computed:            true,
						Default:             stringdefault.StaticString("work"),
						MarkdownDescription: "`backlog`, `work` or `completed`.",
					},
					"color": schema.StringAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "A six digit hex colour, as in `#6d78d5`. Spotlight picks one when it is left out.",
					},
					"description": optional("What the station is for."),
					"harness":     optional("The agent harness that works tasks here, as in `claude-code`."),
					"model":       optional("The model the harness uses."),
					"effort":      optional("How hard the agent thinks: `low`, `medium`, `high`, `xhigh` or `max`."),
					"prompt":      optional("What the agent is asked to do, usually read with `file()`."),
					"check":       optional("A command that must pass before a task moves on, as in `npm test`."),
					"setup":       optional("A command that prepares the workspace before the agent starts."),
					"diffs":       derived("Whether the station's runs draft a change stack from their diff."),
					"decisions":   derived("Whether the station records the agent's decisions for review."),
					"tools":       stringList("The tools the station gives its agent, as in `linear.comment`."),
					"skills":      stringList("The skills the station gives its agent, by key."),
					"passes_to": schema.SetAttribute{
						Optional:            true,
						Computed:            true,
						ElementType:         types.StringType,
						Default:             setdefault.StaticValue(noStringSet),
						MarkdownDescription: "The stations a task here may move on to, by key.",
					},
					"ends": flag("Whether a task that reaches the station is finished.", false),
				},
			},
		},
		"reaction": schema.SetNestedBlock{
			MarkdownDescription: "Moves a task when something happens, as when a pull request's checks fail.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"happening": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "What happens, named source first, as in `github.checks.failed`.",
					},
					"from": optional("The station the task must be at, by key; any station when left out."),
					"to": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "The station the task moves to, by key.",
					},
				},
			},
		},
		"reviews": schema.SingleNestedBlock{
			MarkdownDescription: "Which pull requests the line reviews, for a line whose role is `review`.",
			Attributes: map[string]schema.Attribute{
				"start": schema.StringAttribute{
					Optional:            true,
					Computed:            true,
					Default:             stringdefault.StaticString("mentioned"),
					MarkdownDescription: "`opened`, to review every pull request as it opens, or `mentioned`, to review one when Spotlight is mentioned on it.",
				},
				"mention": schema.StringAttribute{
					Optional:            true,
					Computed:            true,
					Default:             stringdefault.StaticString("@spotlight"),
					MarkdownDescription: "The handle that asks for a review.",
				},
			},
			Blocks: map[string]schema.Block{
				"skip": schema.SingleNestedBlock{
					MarkdownDescription: "Pull requests not to review.",
					Attributes: map[string]schema.Attribute{
						"spotlight": flag("Skip pull requests Spotlight opened.", true),
						"drafts":    flag("Skip draft pull requests.", true),
						"authors":   stringList("Skip pull requests by these GitHub logins, as in `dependabot[bot]`."),
					},
				},
			},
		},
	},
}
