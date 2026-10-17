package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
)

// LineResourceModel is a spotlight_line as Terraform stores it.
type LineResourceModel struct {
	ID           types.String     `tfsdk:"id"`
	Version      types.Int64      `tfsdk:"version"`
	Address      types.String     `tfsdk:"address"`
	TeamID       types.String     `tfsdk:"team_id"`
	Key          types.String     `tfsdk:"key"`
	Name         types.String     `tfsdk:"name"`
	Role         types.String     `tfsdk:"role"`
	Runners      types.Set        `tfsdk:"runners"`
	Repositories []lineRepository `tfsdk:"repository"`
	Stations     []lineStation    `tfsdk:"station"`
	Reactions    []lineReaction   `tfsdk:"reaction"`
	Reviews      *lineReviews     `tfsdk:"reviews"`
	ReplayCheck  *lineReplayCheck `tfsdk:"replay_check"`
	Source       *lineSource      `tfsdk:"source"`
}

type lineRepository struct {
	Name  types.String `tfsdk:"name"`
	Image types.String `tfsdk:"image"`
	Setup types.String `tfsdk:"setup"`
}

type lineStation struct {
	Key         types.String `tfsdk:"key"`
	Name        types.String `tfsdk:"name"`
	Kind        types.String `tfsdk:"kind"`
	Color       types.String `tfsdk:"color"`
	Description types.String `tfsdk:"description"`
	Harness     types.String `tfsdk:"harness"`
	Model       types.String `tfsdk:"model"`
	Effort      types.String `tfsdk:"effort"`
	Profile     types.String `tfsdk:"profile"`
	Prompt      types.String `tfsdk:"prompt"`
	Check       types.String `tfsdk:"check"`
	Diffs       types.Bool   `tfsdk:"diffs"`
	Decisions   types.Bool   `tfsdk:"decisions"`
	Memories    types.Bool   `tfsdk:"memories"`
	Tools       types.List   `tfsdk:"tools"`
	Skills      types.List   `tfsdk:"skills"`
	Runners     types.Set    `tfsdk:"runners"`
	PassesTo    types.Set    `tfsdk:"passes_to"`
	Ends        types.Bool   `tfsdk:"ends"`
	Judge       *lineJudge   `tfsdk:"judge"`
	Alerts      []lineAlert  `tfsdk:"alert"`
}

type lineJudge struct {
	Rubric    types.String  `tfsdk:"rubric"`
	Claims    types.List    `tfsdk:"claims"`
	PassShare types.Float64 `tfsdk:"pass_share"`
	Sample    types.Float64 `tfsdk:"sample"`
	Trust     types.Float64 `tfsdk:"trust"`
}

type lineAlert struct {
	Measure types.String  `tfsdk:"measure"`
	RisesBy types.Float64 `tfsdk:"rises_by"`
	Above   types.Float64 `tfsdk:"above"`
	FallsBy types.Float64 `tfsdk:"falls_by"`
	Below   types.Float64 `tfsdk:"below"`
	Over    types.Int64   `tfsdk:"over"`
}

type lineReaction struct {
	Happening types.String `tfsdk:"happening"`
	From      types.String `tfsdk:"from"`
	To        types.String `tfsdk:"to"`
}

type lineReviews struct {
	Start   types.String `tfsdk:"start"`
	Mention types.String `tfsdk:"mention"`
	Label   types.String `tfsdk:"label"`
	Skip    *lineSkip    `tfsdk:"skip"`
}

type lineSkip struct {
	Spotlight types.Bool `tfsdk:"spotlight"`
	Drafts    types.Bool `tfsdk:"drafts"`
	Authors   types.Set  `tfsdk:"authors"`
}

type lineReplayCheck struct {
	MaxRegressions types.Int64   `tfsdk:"max_regressions"`
	Ceiling        types.Float64 `tfsdk:"ceiling"`
	Currency       types.String  `tfsdk:"currency"`
	MinGain        types.Float64 `tfsdk:"min_gain"`
}

type lineSource struct {
	Repository types.String `tfsdk:"repository"`
	Path       types.String `tfsdk:"path"`
}

type lineWritten map[string]any

func (w lineWritten) text(name string, value types.String) {
	if !value.IsNull() && !value.IsUnknown() {
		w[name] = value.ValueString()
	}
}

func (w lineWritten) flag(name string, value types.Bool) {
	if !value.IsNull() && !value.IsUnknown() {
		w[name] = value.ValueBool()
	}
}

func (w lineWritten) number(name string, value types.Float64) {
	if !value.IsNull() && !value.IsUnknown() {
		w[name] = value.ValueFloat64()
	}
}

func (w lineWritten) count(name string, value types.Int64) {
	if !value.IsNull() && !value.IsUnknown() {
		w[name] = value.ValueInt64()
	}
}

func (w lineWritten) texts(name string, value interface {
	IsNull() bool
	IsUnknown() bool
	ElementsAs(context.Context, any, bool) diag.Diagnostics
}, diags *diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return
	}

	items := []string{}
	diags.Append(value.ElementsAs(context.Background(), &items, false)...)
	w[name] = lineAnyOf(items)
}

func lineAnyOf(items []string) []any {
	out := make([]any, len(items))
	for at, each := range items {
		out[at] = each
	}

	return out
}

func lineInput(plan LineResourceModel) (client.Definition, diag.Diagnostics) {
	var diags diag.Diagnostics

	line := lineWritten{}
	line.text("key", plan.Key)
	line.text("name", plan.Name)
	line.text("role", plan.Role)
	line.texts("runners", plan.Runners, &diags)

	repositories := make([]any, 0, len(plan.Repositories))
	for _, each := range plan.Repositories {
		repository := lineWritten{}
		repository.text("name", each.Name)
		repository.text("image", each.Image)
		repository.text("setup", each.Setup)
		repositories = append(repositories, map[string]any(repository))
	}
	line["repositories"] = repositories

	stations := make([]any, 0, len(plan.Stations))
	for _, each := range plan.Stations {
		station := lineWritten{}
		station.text("key", each.Key)
		station.text("name", each.Name)
		station.text("kind", each.Kind)
		station.text("color", each.Color)
		station.text("description", each.Description)
		station.text("harness", each.Harness)
		station.text("model", each.Model)
		station.text("effort", each.Effort)
		station.text("profile", each.Profile)
		station.text("prompt", each.Prompt)
		station.text("check", each.Check)
		station.flag("diffs", each.Diffs)
		station.flag("decisions", each.Decisions)
		station.flag("memories", each.Memories)
		station.texts("tools", each.Tools, &diags)
		station.texts("skills", each.Skills, &diags)
		station.texts("runners", each.Runners, &diags)
		station.texts("passesTo", each.PassesTo, &diags)
		station.flag("ends", each.Ends)

		if each.Judge != nil {
			judge := lineWritten{}
			judge.text("rubric", each.Judge.Rubric)
			judge.texts("claims", each.Judge.Claims, &diags)
			judge.number("passShare", each.Judge.PassShare)
			judge.number("sample", each.Judge.Sample)
			judge.number("trust", each.Judge.Trust)
			station["judge"] = map[string]any(judge)
		}

		alerts := make([]any, 0, len(each.Alerts))
		for _, alert := range each.Alerts {
			raised := lineWritten{}
			raised.text("measure", alert.Measure)
			raised.number("risesBy", alert.RisesBy)
			raised.number("above", alert.Above)
			raised.number("fallsBy", alert.FallsBy)
			raised.number("below", alert.Below)
			raised.count("over", alert.Over)
			alerts = append(alerts, map[string]any(raised))
		}
		station["alerts"] = alerts

		stations = append(stations, map[string]any(station))
	}
	line["stations"] = stations

	reactions := make([]any, 0, len(plan.Reactions))
	for _, each := range plan.Reactions {
		reaction := lineWritten{}
		reaction.text("happening", each.Happening)
		reaction.text("from", each.From)
		reaction.text("to", each.To)
		reactions = append(reactions, map[string]any(reaction))
	}
	line["reactions"] = reactions

	if plan.Reviews != nil {
		reviews := lineWritten{}
		reviews.text("start", plan.Reviews.Start)
		reviews.text("mention", plan.Reviews.Mention)
		reviews.text("label", plan.Reviews.Label)

		if plan.Reviews.Skip != nil {
			skipped := lineWritten{}
			skipped.flag("spotlight", plan.Reviews.Skip.Spotlight)
			skipped.flag("drafts", plan.Reviews.Skip.Drafts)
			skipped.texts("authors", plan.Reviews.Skip.Authors, &diags)
			reviews["skip"] = map[string]any(skipped)
		}

		line["reviews"] = map[string]any(reviews)
	}

	if plan.ReplayCheck != nil {
		replayCheck := lineWritten{}
		replayCheck.count("maxRegressions", plan.ReplayCheck.MaxRegressions)
		replayCheck.number("ceiling", plan.ReplayCheck.Ceiling)
		replayCheck.text("currency", plan.ReplayCheck.Currency)
		replayCheck.number("minGain", plan.ReplayCheck.MinGain)
		line["replayCheck"] = map[string]any(replayCheck)
	}

	if plan.Source != nil {
		source := lineWritten{}
		source.text("repository", plan.Source.Repository)
		source.text("path", plan.Source.Path)
		line["source"] = map[string]any(source)
	}

	return client.Definition(line), diags
}

type lineRead map[string]any

func (r lineRead) text(name string) types.String {
	if value, ok := r[name].(string); ok {
		return types.StringValue(value)
	}

	return types.StringNull()
}

func (r lineRead) flag(name string) types.Bool {
	if value, ok := r[name].(bool); ok {
		return types.BoolValue(value)
	}

	return types.BoolNull()
}

func (r lineRead) number(name string) types.Float64 {
	value, ok := r[name].(float64)
	if !ok {
		return types.Float64Null()
	}

	return float64AsWritten(value)
}

func (r lineRead) count(name string) types.Int64 {
	if value, ok := r[name].(float64); ok {
		return types.Int64Value(int64(value))
	}

	return types.Int64Null()
}

func (r lineRead) texts(name string) []attr.Value {
	value, _ := r[name].([]any)
	out := make([]attr.Value, 0, len(value))

	for _, each := range value {
		if text, ok := each.(string); ok {
			out = append(out, types.StringValue(text))
		}
	}

	return out
}

func (r lineRead) list(name string) types.List {
	return types.ListValueMust(types.StringType, r.texts(name))
}

func (r lineRead) claims() types.List {
	if _, ok := r["claims"]; !ok {
		return types.ListNull(types.StringType)
	}

	return r.list("claims")
}

func (r lineRead) set(name string) types.Set {
	return types.SetValueMust(types.StringType, r.texts(name))
}

func (r lineRead) each(name string) []lineRead {
	value, _ := r[name].([]any)
	out := make([]lineRead, 0, len(value))

	for _, each := range value {
		if object, ok := each.(map[string]any); ok {
			out = append(out, lineRead(object))
		}
	}

	return out
}

func (r lineRead) object(name string) (lineRead, bool) {
	value, ok := r[name].(map[string]any)

	return lineRead(value), ok
}

func lineAnswered(answer client.Line) LineResourceModel {
	line := lineRead(answer.Definition)

	model := LineResourceModel{
		ID:           types.StringValue(answer.ID),
		Version:      types.Int64Value(answer.Version),
		Address:      types.StringValue(answer.Address),
		TeamID:       types.StringValue(answer.TeamID),
		Key:          types.StringValue(answer.Key),
		Name:         line.text("name"),
		Role:         line.text("role"),
		Runners:      line.set("runners"),
		Repositories: []lineRepository{},
		Stations:     []lineStation{},
		Reactions:    []lineReaction{},
	}

	for _, each := range line.each("repositories") {
		model.Repositories = append(model.Repositories, lineRepository{
			Name:  each.text("name"),
			Image: each.text("image"),
			Setup: each.text("setup"),
		})
	}

	for _, each := range line.each("stations") {
		model.Stations = append(model.Stations, lineStationOf(each))
	}

	for _, each := range line.each("reactions") {
		model.Reactions = append(model.Reactions, lineReaction{
			Happening: each.text("happening"),
			From:      each.text("from"),
			To:        each.text("to"),
		})
	}

	if found, ok := line.object("reviews"); ok {
		model.Reviews = &lineReviews{Start: found.text("start"), Mention: found.text("mention"), Label: found.text("label")}

		if skipped, ok := found.object("skip"); ok {
			model.Reviews.Skip = &lineSkip{
				Spotlight: skipped.flag("spotlight"),
				Drafts:    skipped.flag("drafts"),
				Authors:   skipped.set("authors"),
			}
		}
	}

	if found, ok := line.object("replayCheck"); ok {
		model.ReplayCheck = &lineReplayCheck{
			MaxRegressions: found.count("maxRegressions"),
			Ceiling:        found.number("ceiling"),
			Currency:       found.text("currency"),
			MinGain:        found.number("minGain"),
		}
	}

	if found, ok := line.object("source"); ok {
		model.Source = &lineSource{Repository: found.text("repository"), Path: found.text("path")}
	}

	return model
}

func lineStationOf(read lineRead) lineStation {
	station := lineStation{
		Key:         read.text("key"),
		Name:        read.text("name"),
		Kind:        read.text("kind"),
		Color:       read.text("color"),
		Description: read.text("description"),
		Harness:     read.text("harness"),
		Model:       read.text("model"),
		Effort:      read.text("effort"),
		Profile:     read.text("profile"),
		Prompt:      read.text("prompt"),
		Check:       read.text("check"),
		Diffs:       read.flag("diffs"),
		Decisions:   read.flag("decisions"),
		Memories:    read.flag("memories"),
		Tools:       read.list("tools"),
		Skills:      read.list("skills"),
		Runners:     read.set("runners"),
		PassesTo:    read.set("passesTo"),
		Ends:        read.flag("ends"),
		Alerts:      []lineAlert{},
	}

	if judge, ok := read.object("judge"); ok {
		station.Judge = &lineJudge{
			Rubric:    judge.text("rubric"),
			Claims:    judge.claims(),
			PassShare: judge.number("passShare"),
			Sample:    judge.number("sample"),
			Trust:     judge.number("trust"),
		}
	}

	for _, each := range read.each("alerts") {
		station.Alerts = append(station.Alerts, lineAlert{
			Measure: each.text("measure"),
			RisesBy: each.number("risesBy"),
			Above:   each.number("above"),
			FallsBy: each.number("fallsBy"),
			Below:   each.number("below"),
			Over:    each.count("over"),
		})
	}

	return station
}

func lineModel(answer client.Line, kept *LineResourceModel) LineResourceModel {
	full := lineAnswered(answer)
	if kept == nil {
		return full
	}

	merged := *kept
	merged.ID = full.ID
	merged.Version = full.Version
	merged.Address = full.Address
	merged.TeamID = full.TeamID

	answered := map[string]lineStation{}
	for _, each := range full.Stations {
		answered[each.Key.ValueString()] = each
	}

	merged.Stations = make([]lineStation, len(kept.Stations))
	for at, each := range kept.Stations {
		if found, ok := answered[each.Key.ValueString()]; ok {
			each.Color = lineOrAnswered(each.Color, found.Color)
			each.Diffs = lineOrAnsweredFlag(each.Diffs, found.Diffs)
			each.Decisions = lineOrAnsweredFlag(each.Decisions, found.Decisions)
			each.Memories = lineOrAnsweredFlag(each.Memories, found.Memories)
		}
		merged.Stations[at] = each
	}

	return merged
}

func lineOrAnswered(planned, answered types.String) types.String {
	if planned.IsUnknown() {
		return answered
	}

	return planned
}

func lineOrAnsweredFlag(planned, answered types.Bool) types.Bool {
	if planned.IsUnknown() {
		return answered
	}

	return planned
}
