package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

var (
	_ resource.Resource                = &LineResource{}
	_ resource.ResourceWithConfigure   = &LineResource{}
	_ resource.ResourceWithImportState = &LineResource{}
	_ resource.ResourceWithIdentity    = &LineResource{}
)

// NewLineResource returns the spotlight_line resource.
func NewLineResource() resource.Resource {
	return &LineResource{}
}

// LineResource is the spotlight_line resource.
type LineResource struct {
	client *client.Client
}

// lineBlocks are the API's lists of objects Terraform writes as repeated blocks.
var lineBlocks = map[string]string{
	"repositories": "repository",
	"stations":     "station",
	"reactions":    "reaction",
	"alerts":       "alert",
}

var lineMeasures = []string{"sent_back_rate", "accepted_rate", "checks_failed_rate", "failed_rate", "merged_rate", "cost_per_run"}

var lineFields = fieldpath.Locator{Schema: lineSchema().Type(), Renames: lineBlocks}

func (r *LineResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_line"
}

func (r *LineResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = lineSchema()
}

func lineOptional(description string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, MarkdownDescription: description}
}

func lineFlag(description string, fallback bool) schema.BoolAttribute {
	return schema.BoolAttribute{
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(fallback),
		MarkdownDescription: description,
	}
}

func lineDerived(description string) schema.BoolAttribute {
	return schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: description}
}

func lineShare(description string) schema.Float64Attribute {
	return schema.Float64Attribute{
		Optional:            true,
		Validators:          []validator.Float64{float64validator.Between(0, 1)},
		MarkdownDescription: description,
	}
}

func lineThreshold(description string, also ...validator.Float64) schema.Float64Attribute {
	return schema.Float64Attribute{
		Optional:            true,
		Validators:          append([]validator.Float64{float64validator.AtLeast(0)}, also...),
		MarkdownDescription: description,
	}
}

func lineSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "A line: the stations a team's tasks move along, in order, and how they move. Import it as `<workspace id>/<team key>/<key>`.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute("line"),
			"version": versionAttribute("line"),
			"address": addressAttribute("core/delivery"),
			"team_id": ownerAttribute("team", "line"),
			"key":     fixedAttribute("Names the line within its team.", "line"),
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "What the line is called.",
			},
			"role":    lineOptional("The role the line plays for its team, as in `delivery` or `review`. A role names one line."),
			"runners": stringSetAttribute("Where the line's work runs: the labels of a runner pool or trust, any of which a runner must answer to. A station's own `runners` replace it. Anywhere when left out."),
		},
		Blocks: map[string]schema.Block{
			"repository": schema.SetNestedBlock{
				MarkdownDescription: "A repository the line works on. Spotlight keeps them sorted by name, so they are a set. The repositories of a line name at most one `image`.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The repository, named `owner/name`.",
						},
						"image": lineOptional("The runner image every station of this line runs in, built FROM `ghcr.io/usespotlightai/spotlight-runner`; unset runs on the host's default image."),
						"setup": lineOptional("Runs in the checkout as the agent before each run's agent starts, and before a change stack check wherever a layer changes a lockfile. Every repository says it when a station has a `check` and `diffs`."),
					},
				},
			},
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
						"description": lineOptional("What the station is for."),
						"harness":     lineOptional("The agent harness that works tasks here, as in `claude-code`."),
						"model":       lineOptional("The model the harness uses."),
						"effort":      lineOptional("How hard the agent thinks: `low`, `medium`, `high`, `xhigh` or `max`."),
						"profile":     lineOptional("The harness profile the station's agent runs with, by key, as in `careful`."),
						"prompt":      lineOptional("What the agent is asked to do, usually read with `file()`."),
						"check":       lineOptional("A command that must pass before a task moves on, as in `npm test`."),
						"diffs":       lineDerived("Whether the station's runs draft a change stack from their diff."),
						"decisions":   lineDerived("Whether the station records the agent's decisions for review."),
						"tools":       stringListAttribute("The tools the station gives its agent, as in `linear.comment`."),
						"skills":      stringListAttribute("The skills the station gives its agent, by key."),
						"runners":     stringSetAttribute("Where the station's work runs, in place of the line's `runners`: the labels of a runner pool or trust, any of which a runner must answer to. The line's apply when left out."),
						"passes_to":   stringSetAttribute("The stations a task here may move on to, by key."),
						"ends":        lineFlag("Whether a task that reaches the station is finished.", false),
					},
					Blocks: map[string]schema.Block{
						"judge": schema.SingleNestedBlock{
							MarkdownDescription: "Scores the station's runs against a rubric, for Insights. A judge sets `rubric`, `sample` and `trust`. No run is judged when left out.",
							Validators: []validator.Object{
								objectvalidator.AlsoRequires(
									path.MatchRelative().AtName("rubric"),
									path.MatchRelative().AtName("sample"),
									path.MatchRelative().AtName("trust"),
								),
							},
							Attributes: map[string]schema.Attribute{
								"rubric": schema.StringAttribute{
									Optional:            true,
									Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
									MarkdownDescription: "What a good run looks like, at most 20,000 characters, usually read with `file()`, as in `file(\"judges/implement.md\")`.",
								},
								"sample": lineShare("The share of runs judged, from 0 to 1."),
								"trust":  lineShare("How often the judge must agree with people, from 0 to 1, before Insights shows its scores."),
							},
						},
						"alert": schema.ListNestedBlock{
							MarkdownDescription: "Warns when a measure of the station's runs moves past a threshold. An alert sets exactly one of `rises_by`, `above`, `falls_by` and `below`. For a rate, amounts are shares from 0 to 1, so 0.2 is 20 percentage points; for `cost_per_run`, they are amounts in the workspace's currency.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"measure": schema.StringAttribute{
										Required:            true,
										Validators:          []validator.String{stringvalidator.OneOf(lineMeasures...)},
										MarkdownDescription: "What is watched: `" + strings.Join(lineMeasures, "`, `") + "`.",
									},
									"rises_by": lineThreshold(
										"Alerts when the measure over the last `over` runs is this much higher than over the `over` runs before. For a rate, a share above 0 and at most 1; for `cost_per_run`, an amount above 0.",
										float64validator.ExactlyOneOf(
											path.MatchRelative().AtParent().AtName("above"),
											path.MatchRelative().AtParent().AtName("falls_by"),
											path.MatchRelative().AtParent().AtName("below"),
										),
									),
									"above": lineThreshold("Alerts when the measure across the last `over` runs is higher than this. For a rate, a share from 0 to 1; for `cost_per_run`, an amount of 0 or more."),
									"falls_by": lineThreshold(
										"Alerts when the measure over the last `over` runs is this much lower than over the `over` runs before. For a rate, a share above 0 and at most 1; for `cost_per_run`, an amount above 0.",
									),
									"below": lineThreshold("Alerts when the measure across the last `over` runs is lower than this. For a rate, a share from 0 to 1; for `cost_per_run`, an amount of 0 or more."),
									"over": schema.Int64Attribute{
										Required:            true,
										Validators:          []validator.Int64{int64validator.AtLeast(1)},
										MarkdownDescription: "How many runs are compared, at least 1.",
									},
								},
							},
						},
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
						"from": lineOptional("The station the task must be at, by key; any station when left out."),
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
							"spotlight": lineFlag("Skip pull requests Spotlight opened.", true),
							"drafts":    lineFlag("Skip draft pull requests.", true),
							"authors":   stringSetAttribute("Skip pull requests by these GitHub logins, in lower case, as in `dependabot[bot]`. Spotlight keeps them sorted and each once, so they are a set."),
						},
					},
				},
			},
		},
	}
}

func (r *LineResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("line")
}

func (r *LineResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *LineResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan LineResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := lineInput(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	line, err := r.client.CreateLine(ctx, plan.TeamID.ValueString(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not create the line", err, lineFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "line", line.Warnings, lineFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, lineModel(line, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, line.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, lineSettable(line))...)
}

func (r *LineResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state LineResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	line, err := r.client.ReadLine(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		removed(ctx, resp, state.ID.ValueString())
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not read the line", err, lineFields, client.ScopeRead)
		return
	}

	kept := &state
	if !answerUnchanged(ctx, req.Private, lineSettable(line)) {
		kept = nil
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, lineModel(line, kept))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, line.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, lineSettable(line))...)
}

func (r *LineResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state LineResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := lineInput(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	line, err := r.client.UpdateLine(ctx, state.ID.ValueString(), state.Version.ValueInt64(), in)
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not update the line", err, lineFields, client.ScopeConfigure)
		return
	}

	addAPIWarnings(&resp.Diagnostics, "line", line.Warnings, lineFields)
	resp.Diagnostics.Append(resp.State.Set(ctx, lineModel(line, &plan))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, line.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, lineSettable(line))...)
}

func (r *LineResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state LineResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteLine(ctx, state.ID.ValueString(), state.Version.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		addAPIError(&resp.Diagnostics, "Could not delete the line", err, lineFields, client.ScopeConfigure)
	}
}

func (r *LineResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, ok := importTargetOf(ctx, req, &resp.Diagnostics, "<workspace id>/core/delivery")
	if !ok {
		return
	}

	var line client.Line
	var err error
	if target.byID() {
		line, err = r.client.ReadLine(ctx, target.id)
	} else {
		line, err = r.client.ImportLine(ctx, target.workspaceID, target.address)
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "Could not import the line at "+target.String(), err, fieldpath.Locator{}, client.ScopeRead)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, lineModel(line, nil))...)
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, line.ID)...)
	resp.Diagnostics.Append(rememberAnswer(ctx, resp.Private, lineSettable(line))...)
}

func lineSettable(line client.Line) any {
	return line.Definition
}
