package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
)

// LineResourceModel is a spotlight_line as Terraform holds it.
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
	Tools       types.List   `tfsdk:"tools"`
	Skills      types.List   `tfsdk:"skills"`
	Runners     types.Set    `tfsdk:"runners"`
	PassesTo    types.Set    `tfsdk:"passes_to"`
	Ends        types.Bool   `tfsdk:"ends"`
}

type lineReaction struct {
	Happening types.String `tfsdk:"happening"`
	From      types.String `tfsdk:"from"`
	To        types.String `tfsdk:"to"`
}

type lineReviews struct {
	Start   types.String `tfsdk:"start"`
	Mention types.String `tfsdk:"mention"`
	Skip    *lineSkip    `tfsdk:"skip"`
}

type lineSkip struct {
	Spotlight types.Bool `tfsdk:"spotlight"`
	Drafts    types.Bool `tfsdk:"drafts"`
	Authors   types.Set  `tfsdk:"authors"`
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

func (w lineWritten) texts(name string, value interface {
	IsNull() bool
	IsUnknown() bool
	ElementsAs(context.Context, any, bool) diag.Diagnostics
}, diags *diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return
	}

	held := []string{}
	diags.Append(value.ElementsAs(context.Background(), &held, false)...)
	w[name] = lineAnyOf(held)
}

func lineAnyOf(held []string) []any {
	out := make([]any, len(held))
	for at, each := range held {
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
		held := lineWritten{}
		held.text("name", each.Name)
		held.text("image", each.Image)
		held.text("setup", each.Setup)
		repositories = append(repositories, map[string]any(held))
	}
	line["repositories"] = repositories

	stations := make([]any, 0, len(plan.Stations))
	for _, each := range plan.Stations {
		held := lineWritten{}
		held.text("key", each.Key)
		held.text("name", each.Name)
		held.text("kind", each.Kind)
		held.text("color", each.Color)
		held.text("description", each.Description)
		held.text("harness", each.Harness)
		held.text("model", each.Model)
		held.text("effort", each.Effort)
		held.text("profile", each.Profile)
		held.text("prompt", each.Prompt)
		held.text("check", each.Check)
		held.flag("diffs", each.Diffs)
		held.flag("decisions", each.Decisions)
		held.texts("tools", each.Tools, &diags)
		held.texts("skills", each.Skills, &diags)
		held.texts("runners", each.Runners, &diags)
		held.texts("passesTo", each.PassesTo, &diags)
		held.flag("ends", each.Ends)
		stations = append(stations, map[string]any(held))
	}
	line["stations"] = stations

	reactions := make([]any, 0, len(plan.Reactions))
	for _, each := range plan.Reactions {
		held := lineWritten{}
		held.text("happening", each.Happening)
		held.text("from", each.From)
		held.text("to", each.To)
		reactions = append(reactions, map[string]any(held))
	}
	line["reactions"] = reactions

	if plan.Reviews != nil {
		held := lineWritten{}
		held.text("start", plan.Reviews.Start)
		held.text("mention", plan.Reviews.Mention)

		if plan.Reviews.Skip != nil {
			skipped := lineWritten{}
			skipped.flag("spotlight", plan.Reviews.Skip.Spotlight)
			skipped.flag("drafts", plan.Reviews.Skip.Drafts)
			skipped.texts("authors", plan.Reviews.Skip.Authors, &diags)
			held["skip"] = map[string]any(skipped)
		}

		line["reviews"] = map[string]any(held)
	}

	return client.Definition(line), diags
}

type lineRead map[string]any

func (r lineRead) text(name string) types.String {
	if held, ok := r[name].(string); ok {
		return types.StringValue(held)
	}

	return types.StringNull()
}

func (r lineRead) flag(name string) types.Bool {
	if held, ok := r[name].(bool); ok {
		return types.BoolValue(held)
	}

	return types.BoolNull()
}

func (r lineRead) texts(name string) []attr.Value {
	held, _ := r[name].([]any)
	out := make([]attr.Value, 0, len(held))

	for _, each := range held {
		if text, ok := each.(string); ok {
			out = append(out, types.StringValue(text))
		}
	}

	return out
}

func (r lineRead) list(name string) types.List {
	return types.ListValueMust(types.StringType, r.texts(name))
}

func (r lineRead) set(name string) types.Set {
	return types.SetValueMust(types.StringType, r.texts(name))
}

func (r lineRead) each(name string) []lineRead {
	held, _ := r[name].([]any)
	out := make([]lineRead, 0, len(held))

	for _, each := range held {
		if object, ok := each.(map[string]any); ok {
			out = append(out, lineRead(object))
		}
	}

	return out
}

func (r lineRead) object(name string) (lineRead, bool) {
	held, ok := r[name].(map[string]any)

	return lineRead(held), ok
}

func lineAnswered(answer client.Line) LineResourceModel {
	line := lineRead(answer.Definition)

	held := LineResourceModel{
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
		held.Repositories = append(held.Repositories, lineRepository{
			Name:  each.text("name"),
			Image: each.text("image"),
			Setup: each.text("setup"),
		})
	}

	for _, each := range line.each("stations") {
		held.Stations = append(held.Stations, lineStationOf(each))
	}

	for _, each := range line.each("reactions") {
		held.Reactions = append(held.Reactions, lineReaction{
			Happening: each.text("happening"),
			From:      each.text("from"),
			To:        each.text("to"),
		})
	}

	if found, ok := line.object("reviews"); ok {
		held.Reviews = &lineReviews{Start: found.text("start"), Mention: found.text("mention")}

		if skipped, ok := found.object("skip"); ok {
			held.Reviews.Skip = &lineSkip{
				Spotlight: skipped.flag("spotlight"),
				Drafts:    skipped.flag("drafts"),
				Authors:   skipped.set("authors"),
			}
		}
	}

	return held
}

func lineStationOf(held lineRead) lineStation {
	return lineStation{
		Key:         held.text("key"),
		Name:        held.text("name"),
		Kind:        held.text("kind"),
		Color:       held.text("color"),
		Description: held.text("description"),
		Harness:     held.text("harness"),
		Model:       held.text("model"),
		Effort:      held.text("effort"),
		Profile:     held.text("profile"),
		Prompt:      held.text("prompt"),
		Check:       held.text("check"),
		Diffs:       held.flag("diffs"),
		Decisions:   held.flag("decisions"),
		Tools:       held.list("tools"),
		Skills:      held.list("skills"),
		Runners:     held.set("runners"),
		PassesTo:    held.set("passesTo"),
		Ends:        held.flag("ends"),
	}
}

func lineModel(answer client.Line, kept *LineResourceModel) LineResourceModel {
	full := lineAnswered(answer)
	if kept == nil {
		return full
	}

	held := *kept
	held.ID = full.ID
	held.Version = full.Version
	held.Address = full.Address
	held.TeamID = full.TeamID

	answered := map[string]lineStation{}
	for _, each := range full.Stations {
		answered[each.Key.ValueString()] = each
	}

	held.Stations = make([]lineStation, len(kept.Stations))
	for at, each := range kept.Stations {
		if found, ok := answered[each.Key.ValueString()]; ok {
			each.Color = lineOrAnswered(each.Color, found.Color)
			each.Diffs = lineOrAnsweredFlag(each.Diffs, found.Diffs)
			each.Decisions = lineOrAnsweredFlag(each.Decisions, found.Decisions)
		}
		held.Stations[at] = each
	}

	return held
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
