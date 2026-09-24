package line

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

type model struct {
	ID           types.String `tfsdk:"id"`
	Version      types.Int64  `tfsdk:"version"`
	Address      types.String `tfsdk:"address"`
	TeamID       types.String `tfsdk:"team_id"`
	Key          types.String `tfsdk:"key"`
	Name         types.String `tfsdk:"name"`
	Role         types.String `tfsdk:"role"`
	Repositories types.Set    `tfsdk:"repositories"`
	Runners      types.Set    `tfsdk:"runners"`
	Stations     []station    `tfsdk:"station"`
	Reactions    []reaction   `tfsdk:"reaction"`
	Reviews      *reviews     `tfsdk:"reviews"`
}

type station struct {
	Key            types.String `tfsdk:"key"`
	Name           types.String `tfsdk:"name"`
	Kind           types.String `tfsdk:"kind"`
	Color          types.String `tfsdk:"color"`
	Description    types.String `tfsdk:"description"`
	Harness        types.String `tfsdk:"harness"`
	Model          types.String `tfsdk:"model"`
	Effort         types.String `tfsdk:"effort"`
	Profile        types.String `tfsdk:"profile"`
	HarnessVersion types.String `tfsdk:"harness_version"`
	Prompt         types.String `tfsdk:"prompt"`
	Check          types.String `tfsdk:"check"`
	Setup          types.String `tfsdk:"setup"`
	Diffs          types.Bool   `tfsdk:"diffs"`
	Decisions      types.Bool   `tfsdk:"decisions"`
	Tools          types.List   `tfsdk:"tools"`
	Skills         types.List   `tfsdk:"skills"`
	Runners        types.Set    `tfsdk:"runners"`
	PassesTo       types.Set    `tfsdk:"passes_to"`
	Ends           types.Bool   `tfsdk:"ends"`
}

type reaction struct {
	Happening types.String `tfsdk:"happening"`
	From      types.String `tfsdk:"from"`
	To        types.String `tfsdk:"to"`
}

type reviews struct {
	Start   types.String `tfsdk:"start"`
	Mention types.String `tfsdk:"mention"`
	Skip    *skip        `tfsdk:"skip"`
}

type skip struct {
	Spotlight types.Bool `tfsdk:"spotlight"`
	Drafts    types.Bool `tfsdk:"drafts"`
	Authors   types.Set  `tfsdk:"authors"`
}

type written map[string]any

func (w written) text(name string, value types.String) {
	if !value.IsNull() && !value.IsUnknown() {
		w[name] = value.ValueString()
	}
}

func (w written) flag(name string, value types.Bool) {
	if !value.IsNull() && !value.IsUnknown() {
		w[name] = value.ValueBool()
	}
}

func (w written) texts(name string, value interface {
	IsNull() bool
	IsUnknown() bool
	ElementsAs(context.Context, any, bool) diag.Diagnostics
}, diags *diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return
	}

	held := []string{}
	diags.Append(value.ElementsAs(context.Background(), &held, false)...)
	w[name] = toAny(held)
}

func toAny(held []string) []any {
	out := make([]any, len(held))
	for at, each := range held {
		out[at] = each
	}

	return out
}

func definitionOf(plan model) (spotlight.Definition, diag.Diagnostics) {
	var diags diag.Diagnostics

	line := written{}
	line.text("key", plan.Key)
	line.text("name", plan.Name)
	line.text("role", plan.Role)
	line.texts("repositories", plan.Repositories, &diags)
	line.texts("runners", plan.Runners, &diags)

	stations := make([]any, 0, len(plan.Stations))
	for _, each := range plan.Stations {
		held := written{}
		held.text("key", each.Key)
		held.text("name", each.Name)
		held.text("kind", each.Kind)
		held.text("color", each.Color)
		held.text("description", each.Description)
		held.text("harness", each.Harness)
		held.text("model", each.Model)
		held.text("effort", each.Effort)
		held.text("profile", each.Profile)
		held.text("harnessVersion", each.HarnessVersion)
		held.text("prompt", each.Prompt)
		held.text("check", each.Check)
		held.text("setup", each.Setup)
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
		held := written{}
		held.text("happening", each.Happening)
		held.text("from", each.From)
		held.text("to", each.To)
		reactions = append(reactions, map[string]any(held))
	}
	line["reactions"] = reactions

	if plan.Reviews != nil {
		held := written{}
		held.text("start", plan.Reviews.Start)
		held.text("mention", plan.Reviews.Mention)

		if plan.Reviews.Skip != nil {
			skipped := written{}
			skipped.flag("spotlight", plan.Reviews.Skip.Spotlight)
			skipped.flag("drafts", plan.Reviews.Skip.Drafts)
			skipped.texts("authors", plan.Reviews.Skip.Authors, &diags)
			held["skip"] = map[string]any(skipped)
		}

		line["reviews"] = map[string]any(held)
	}

	return spotlight.Definition(line), diags
}

type read map[string]any

func (r read) text(name string) types.String {
	if held, ok := r[name].(string); ok {
		return types.StringValue(held)
	}

	return types.StringNull()
}

func (r read) flag(name string) types.Bool {
	if held, ok := r[name].(bool); ok {
		return types.BoolValue(held)
	}

	return types.BoolNull()
}

func (r read) texts(name string) []attr.Value {
	held, _ := r[name].([]any)
	out := make([]attr.Value, 0, len(held))

	for _, each := range held {
		if text, ok := each.(string); ok {
			out = append(out, types.StringValue(text))
		}
	}

	return out
}

func (r read) list(name string) types.List {
	return types.ListValueMust(types.StringType, r.texts(name))
}

func (r read) set(name string) types.Set {
	return types.SetValueMust(types.StringType, r.texts(name))
}

func (r read) each(name string) []read {
	held, _ := r[name].([]any)
	out := make([]read, 0, len(held))

	for _, each := range held {
		if object, ok := each.(map[string]any); ok {
			out = append(out, read(object))
		}
	}

	return out
}

func (r read) object(name string) (read, bool) {
	held, ok := r[name].(map[string]any)

	return read(held), ok
}

func modelOf(answer spotlight.Line) model {
	line := read(answer.Definition)

	held := model{
		ID:           types.StringValue(answer.ID),
		Version:      types.Int64Value(answer.Version),
		Address:      types.StringValue(answer.Address),
		TeamID:       types.StringValue(answer.TeamID),
		Key:          types.StringValue(answer.Key),
		Name:         line.text("name"),
		Role:         line.text("role"),
		Repositories: line.set("repositories"),
		Runners:      line.set("runners"),
		Stations:     []station{},
		Reactions:    []reaction{},
	}

	for _, each := range line.each("stations") {
		held.Stations = append(held.Stations, stationOf(each))
	}

	for _, each := range line.each("reactions") {
		held.Reactions = append(held.Reactions, reaction{
			Happening: each.text("happening"),
			From:      each.text("from"),
			To:        each.text("to"),
		})
	}

	if found, ok := line.object("reviews"); ok {
		held.Reviews = &reviews{Start: found.text("start"), Mention: found.text("mention")}

		if skipped, ok := found.object("skip"); ok {
			held.Reviews.Skip = &skip{
				Spotlight: skipped.flag("spotlight"),
				Drafts:    skipped.flag("drafts"),
				Authors:   skipped.set("authors"),
			}
		}
	}

	return held
}

func stationOf(held read) station {
	return station{
		Key:            held.text("key"),
		Name:           held.text("name"),
		Kind:           held.text("kind"),
		Color:          held.text("color"),
		Description:    held.text("description"),
		Harness:        held.text("harness"),
		Model:          held.text("model"),
		Effort:         held.text("effort"),
		Profile:        held.text("profile"),
		HarnessVersion: held.text("harnessVersion"),
		Prompt:         held.text("prompt"),
		Check:          held.text("check"),
		Setup:          held.text("setup"),
		Diffs:          held.flag("diffs"),
		Decisions:      held.flag("decisions"),
		Tools:          held.list("tools"),
		Skills:         held.list("skills"),
		Runners:        held.set("runners"),
		PassesTo:       held.set("passesTo"),
		Ends:           held.flag("ends"),
	}
}

func stored(answer spotlight.Line, kept *model) (model, diag.Diagnostics) {
	full := modelOf(answer)
	if kept == nil {
		return full, nil
	}

	held := *kept
	held.ID = full.ID
	held.Version = full.Version
	held.Address = full.Address
	held.TeamID = full.TeamID

	answered := map[string]station{}
	for _, each := range full.Stations {
		answered[each.Key.ValueString()] = each
	}

	held.Stations = make([]station, len(kept.Stations))
	for at, each := range kept.Stations {
		if found, ok := answered[each.Key.ValueString()]; ok {
			each.Color = orAnswered(each.Color, found.Color)
			each.Diffs = orAnsweredFlag(each.Diffs, found.Diffs)
			each.Decisions = orAnsweredFlag(each.Decisions, found.Decisions)
		}
		held.Stations[at] = each
	}

	return held, nil
}

func orAnswered(planned, answered types.String) types.String {
	if planned.IsUnknown() {
		return answered
	}

	return planned
}

func orAnsweredFlag(planned, answered types.Bool) types.Bool {
	if planned.IsUnknown() {
		return answered
	}

	return planned
}
