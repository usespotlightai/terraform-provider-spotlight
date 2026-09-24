package fakeapi

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

var reserved = map[string]bool{"review": true, "done": true}

func placed(held map[string]any, field, at string, errs *[]fieldError) {
	selectors := texts(held[field])
	if len(selectors) == 0 {
		delete(held, field)
		return
	}

	seen := map[string]bool{}
	for _, each := range selectors {
		if seen[each.(string)] {
			*errs = append(*errs, fieldError{at, fmt.Sprintf("name %s only once", each)})
		}
		seen[each.(string)] = true
	}

	held[field] = selectors
}

var colors = map[string]string{
	"backlog":   "#a3a8b0",
	"work":      "#6d78d5",
	"completed": "#3fa46a",
}

func normalized(kind string, body map[string]any) (map[string]any, []fieldError) {
	switch kind {
	case "workspace":
		return named(body, "slug", "name")
	case "team":
		return named(body, "key", "name")
	case "line":
		return line(body)
	case "skill":
		return skill(body)
	case "harness-profile":
		return harnessProfile(body)
	case "secret":
		return secret(body)
	case "runner":
		return runner(body)
	case "runner-trust":
		return runnerTrust(body)
	case "workspace-runners":
		return workspaceRunners(body)
	default:
		return integration(body)
	}
}

func named(body map[string]any, fields ...string) (map[string]any, []fieldError) {
	out := map[string]any{}
	var errs []fieldError

	for _, field := range fields {
		text, _ := body[field].(string)
		if text == "" {
			errs = append(errs, fieldError{field, "must not be empty"})
		}
		out[field] = text
	}

	return out, errs
}

func texts(held any) []any {
	listed, _ := held.([]any)
	out := make([]any, 0, len(listed))

	for _, each := range listed {
		if text, ok := each.(string); ok {
			out = append(out, text)
		}
	}

	return out
}

func orDefault(held map[string]any, field string, fallback any) {
	if _, ok := held[field]; !ok {
		held[field] = fallback
	}
}

func line(body map[string]any) (map[string]any, []fieldError) {
	definition := maps.Clone(body)
	var errs []fieldError

	for _, field := range []string{"key", "name"} {
		if text, _ := definition[field].(string); text == "" {
			errs = append(errs, fieldError{field, "must not be empty"})
		}
	}

	orDefault(definition, "repositories", []any{})
	orDefault(definition, "reactions", []any{})

	listed, _ := definition["stations"].([]any)
	if len(listed) == 0 {
		errs = append(errs, fieldError{"stations", "is the stations along this line, in order, and there has to be one"})
	}

	keys, worked := map[string]bool{}, map[string]bool{}
	for at, each := range listed {
		if station, ok := each.(map[string]any); ok {
			key, _ := station["key"].(string)
			keys[key] = true
			if kind, _ := station["kind"].(string); kind == "" || kind == "work" {
				worked[key] = true
			}
			if reserved[key] {
				errs = append(errs, fieldError{fmt.Sprintf("stations[%d].key", at), fmt.Sprintf("cannot be %s, which is reserved because a run says %s; pick another key, as in code-review", key, key)})
			}
		}
	}

	placed(definition, "runners", "runners", &errs)

	stations := make([]any, 0, len(listed))
	for at, each := range listed {
		station := maps.Clone(each.(map[string]any))
		orDefault(station, "kind", "work")
		orDefault(station, "color", colors[station["kind"].(string)])
		orDefault(station, "diffs", false)
		orDefault(station, "decisions", false)
		orDefault(station, "tools", []any{})
		orDefault(station, "skills", []any{})
		orDefault(station, "passesTo", []any{})
		orDefault(station, "ends", false)

		for _, next := range texts(station["passesTo"]) {
			switch {
			case !keys[next.(string)]:
				errs = append(errs, fieldError{fmt.Sprintf("stations[%d].passesTo", at), fmt.Sprintf("names %s, which is not a station on this line", next)})
			case !worked[next.(string)]:
				errs = append(errs, fieldError{fmt.Sprintf("stations[%d].passesTo", at), fmt.Sprintf("must hand on to a station work is done at, but %s is not one", next)})
			}
		}

		placed(station, "runners", fmt.Sprintf("stations[%d].runners", at), &errs)

		if version, ok := station["harnessVersion"].(string); ok {
			station["harnessVersion"] = strings.TrimPrefix(strings.TrimSpace(version), "v")
		}

		stations = append(stations, station)
	}
	definition["stations"] = stations

	for at, each := range definition["reactions"].([]any) {
		reaction, _ := each.(map[string]any)
		if to, _ := reaction["to"].(string); !keys[to] {
			errs = append(errs, fieldError{fmt.Sprintf("reactions[%d].to", at), fmt.Sprintf("names %s, which is not a station on this line", to)})
		}
	}

	if reviews, ok := definition["reviews"].(map[string]any); ok {
		if definition["role"] != "review" {
			errs = append(errs, fieldError{"reviews", fmt.Sprintf("The %s line reviews pull requests, which only a line whose role is review does", definition["key"])})
		}

		reviews = maps.Clone(reviews)
		orDefault(reviews, "start", "mentioned")
		orDefault(reviews, "mention", "@spotlight")
		orDefault(reviews, "skip", map[string]any{})

		skip := maps.Clone(reviews["skip"].(map[string]any))
		orDefault(skip, "spotlight", true)
		orDefault(skip, "drafts", true)

		authors := []string{}
		for _, each := range texts(skip["authors"]) {
			authors = append(authors, strings.ToLower(each.(string)))
		}
		slices.Sort(authors)
		skip["authors"] = slices.Compact(authors)

		reviews["skip"] = skip
		definition["reviews"] = reviews
	}

	return map[string]any{"key": definition["key"], "definition": definition}, errs
}

func skill(body map[string]any) (map[string]any, []fieldError) {
	key, _ := body["key"].(string)
	files, _ := body["files"].(map[string]any)

	var errs []fieldError
	if key == "" {
		errs = append(errs, fieldError{"key", "must not be empty"})
	}

	manual, ok := files["SKILL.md"].(string)
	if !ok {
		errs = append(errs, fieldError{"files", `holds a SKILL.md`})
	}

	description := ""
	for _, text := range strings.Split(manual, "\n") {
		if found, ok := strings.CutPrefix(text, "description:"); ok {
			description = strings.TrimSpace(found)
		}
	}

	return map[string]any{
		"key":         key,
		"files":       files,
		"description": description,
		"secrets":     []any{},
		"tools":       []any{},
	}, errs
}

func integration(body map[string]any) (map[string]any, []fieldError) {
	definition := maps.Clone(body)

	source, _ := definition["integration"].(string)
	if source == "" {
		return nil, []fieldError{{"integration", "names the host this integration defines, as in linear"}}
	}

	if actions, ok := definition["actions"].(map[string]any); ok {
		filled := map[string]any{}
		for name, each := range actions {
			action := maps.Clone(each.(map[string]any))
			if request, ok := action["request"].(map[string]any); ok {
				request = maps.Clone(request)
				orDefault(request, "method", "POST")
				orDefault(request, "headers", map[string]any{})
				action["request"] = request
			}
			filled[name] = action
		}
		definition["actions"] = filled
	}

	return map[string]any{"source": source, "definition": definition}, nil
}
