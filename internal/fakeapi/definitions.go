package fakeapi

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

var reserved = map[string]bool{"review": true, "done": true}

func placed(object map[string]any, field, at string, errs *[]fieldError) {
	selectors := texts(object[field])
	if len(selectors) == 0 {
		delete(object, field)
		return
	}

	seen := map[string]bool{}
	for _, each := range selectors {
		if seen[textOf(each)] {
			*errs = append(*errs, fieldError{at, fmt.Sprintf("name %s only once", each)})
		}
		seen[textOf(each)] = true
	}

	object[field] = selectors
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
	case "runner-trust":
		return runnerTrust(body)
	case "workspace-token-trust":
		return tokenTrust(body)
	case "runner-pool":
		return runnerPool(body)
	case "egress":
		return egress(body)
	case "workspace-runners":
		return workspaceRunners(body)
	case "hook":
		return hook(body)
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

func texts(value any) []any {
	listed, _ := value.([]any)
	out := make([]any, 0, len(listed))

	for _, each := range listed {
		if text, ok := each.(string); ok {
			out = append(out, text)
		}
	}

	return out
}

func orDefault(object map[string]any, field string, fallback any) {
	if _, ok := object[field]; !ok {
		object[field] = fallback
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
		station := maps.Clone(objectOf(each))
		orDefault(station, "kind", "work")
		orDefault(station, "color", colors[textOf(station["kind"])])
		orDefault(station, "diffs", false)
		orDefault(station, "decisions", false)
		orDefault(station, "memories", false)
		orDefault(station, "tools", []any{})
		orDefault(station, "skills", []any{})
		orDefault(station, "passesTo", []any{})
		orDefault(station, "ends", false)
		orDefault(station, "alerts", []any{})

		for _, next := range texts(station["passesTo"]) {
			switch {
			case !keys[textOf(next)]:
				errs = append(errs, fieldError{fmt.Sprintf("stations[%d].passesTo", at), fmt.Sprintf("names %s, which is not a station on this line", next)})
			case !worked[textOf(next)]:
				errs = append(errs, fieldError{fmt.Sprintf("stations[%d].passesTo", at), fmt.Sprintf("must pass to work stations, but %s is not one", next)})
			}
		}

		placed(station, "runners", fmt.Sprintf("stations[%d].runners", at), &errs)
		judged(station, at, &errs)

		stations = append(stations, station)
	}
	definition["stations"] = stations

	repositories(definition, &errs)

	for at, each := range listOf(definition["reactions"]) {
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
		orDefault(reviews, "label", "spotlight")
		orDefault(reviews, "skip", map[string]any{})

		skip := maps.Clone(objectOf(reviews["skip"]))
		orDefault(skip, "spotlight", true)
		orDefault(skip, "drafts", true)

		authors := []string{}
		for _, each := range texts(skip["authors"]) {
			authors = append(authors, strings.ToLower(textOf(each)))
		}
		slices.Sort(authors)
		skip["authors"] = slices.Compact(authors)

		reviews["skip"] = skip
		definition["reviews"] = reviews
	}

	if check, ok := definition["replayCheck"].(map[string]any); ok {
		check = maps.Clone(check)
		orDefault(check, "maxRegressions", float64(0))
		orDefault(check, "ceiling", float64(10))
		orDefault(check, "currency", "USD")
		orDefault(check, "minGain", 0.05)
		check["currency"] = strings.ToUpper(strings.TrimSpace(textOf(check["currency"])))
		if ceiling, _ := check["ceiling"].(float64); ceiling <= 0 || ceiling > 1000 {
			errs = append(errs, fieldError{"replayCheck.ceiling", "is the most one check’s replay may spend, above 0 and at most 1,000"})
		}
		if gain, _ := check["minGain"].(float64); gain <= 0 || gain > 1 {
			errs = append(errs, fieldError{"replayCheck.minGain", "is the smallest change in success rate worth acting on, above 0 and at most 1"})
		}
		definition["replayCheck"] = check
	}

	if source, ok := definition["source"].(map[string]any); ok {
		source = maps.Clone(source)
		source["repository"] = strings.TrimSuffix(strings.TrimSpace(textOf(source["repository"])), ".git")
		if path := textOf(source["path"]); !strings.HasSuffix(path, ".tf") {
			errs = append(errs, fieldError{"source.path", "is a Terraform file, ending in .tf"})
		}
		definition["source"] = source
	} else {
		delete(definition, "source")
	}

	return map[string]any{"key": definition["key"], "definition": definition}, errs
}

var imageReference = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*(:[0-9]+)?(/[a-z0-9]+([._-][a-z0-9]+)*)*(:[A-Za-z0-9_][A-Za-z0-9._-]{0,127})?(@sha256:[0-9a-f]{64})?$`)

func judged(station map[string]any, at int, errs *[]fieldError) {
	judge, ok := station["judge"].(map[string]any)
	if !ok {
		return
	}

	judge = maps.Clone(judge)
	field := fmt.Sprintf("stations[%d].judge", at)
	claims, claimed := judge["claims"].([]any)

	if !claimed && textOf(judge["rubric"]) == "" {
		*errs = append(*errs, fieldError{field, "needs a rubric, claims, or both"})
	}

	if claimed {
		orDefault(judge, "passShare", float64(1))
		if len(claims) < 1 || len(claims) > 20 {
			*errs = append(*errs, fieldError{field + ".claims", "is the checkable claims the judge answers yes or no, from 1 to 20"})
		}
		for each, claim := range claims {
			if text := textOf(claim); text == "" || len([]rune(text)) > 500 {
				*errs = append(*errs, fieldError{fmt.Sprintf("%s.claims[%d]", field, each), "is a checkable claim from 1 to 500 characters"})
			}
		}
	} else if _, ok := judge["passShare"]; ok {
		*errs = append(*errs, fieldError{field + ".passShare", "is only set with claims"})
	}

	if share, ok := judge["passShare"].(float64); ok && (share < 0 || share > 1) {
		*errs = append(*errs, fieldError{field + ".passShare", "is the share of claims a change must meet to pass, from 0 to 1"})
	}

	station["judge"] = judge
}

func repositories(definition map[string]any, errs *[]fieldError) {
	listed := listOf(definition["repositories"])
	worked := make([]map[string]any, 0, len(listed))
	var images []string
	seen := map[string]bool{}

	for at, each := range listed {
		given := objectOf(each)
		name := strings.TrimSpace(textOf(given["name"]))
		if owner, rest, ok := strings.Cut(name, "/"); !ok || owner == "" || rest == "" || strings.Contains(rest, "/") {
			*errs = append(*errs, fieldError{fmt.Sprintf("repositories[%d].name", at), "A repository is named owner/name"})
		}
		if seen[name] {
			*errs = append(*errs, fieldError{"repositories", fmt.Sprintf("The %s line works in repositories that A line works %s twice, and each is cloned into a directory of its own name", definition["key"], name)})
		}
		seen[name] = true

		repository := map[string]any{"name": name}
		if image := strings.TrimSpace(textOf(given["image"])); image != "" {
			if len(image) > 255 || !imageReference.MatchString(image) {
				*errs = append(*errs, fieldError{fmt.Sprintf("repositories[%d].image", at), "A runner image is named by a reference such as ghcr.io/acme/runner:main, in lowercase, with an optional tag or digest"})
			}
			if !slices.Contains(images, image) {
				images = append(images, image)
			}
			repository["image"] = image
		}
		if setup := textOf(given["setup"]); setup != "" {
			if command := strings.TrimSpace(setup); command == "" || len(command) > 500 || strings.ContainsAny(command, "\r\n") {
				*errs = append(*errs, fieldError{fmt.Sprintf("repositories[%d].setup", at), "Invalid repository setup: is one line of at most 500 characters"})
			}
			repository["setup"] = setup
		}
		worked = append(worked, repository)
	}

	if len(images) > 1 {
		*errs = append(*errs, fieldError{"repositories", fmt.Sprintf("The %s line works in repositories that A line runs every station in one image, but its repositories name %s", definition["key"], strings.Join(images, " and "))})
	}

	stations := listOf(definition["stations"])
	checking := slices.IndexFunc(stations, func(each any) bool {
		return textOf(objectOf(each)["check"]) != "" && objectOf(each)["diffs"] == true
	})
	unready := slices.IndexFunc(worked, func(each map[string]any) bool { return each["setup"] == nil })
	if checking >= 0 && unready >= 0 {
		*errs = append(*errs, fieldError{fmt.Sprintf("repositories[%d].setup", unready), fmt.Sprintf("The %s line has %s, which checks its layers, so %s has to say how its checkout is made ready", definition["key"], objectOf(stations[checking])["key"], worked[unready]["name"])})
	}

	slices.SortFunc(worked, func(one, other map[string]any) int {
		return strings.Compare(textOf(one["name"]), textOf(other["name"]))
	})

	sorted := make([]any, len(worked))
	for at, each := range worked {
		sorted[at] = each
	}
	definition["repositories"] = sorted
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
		errs = append(errs, fieldError{"files", `contains a SKILL.md`})
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
			action := maps.Clone(objectOf(each))
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

var generated = map[string][]string{
	"openapi": {"find-pets", "add-pet"},
	"graphql": {"viewer", "create-issue"},
	"mcp":     {"search-issues", "delete-issue"},
}

func describedBy(fields map[string]any) string {
	definition := objectOf(fields["definition"])
	for _, kind := range slices.Sorted(maps.Keys(generated)) {
		if definition[kind] != nil {
			return kind
		}
	}

	return ""
}

func offered(fields map[string]any) map[string]any {
	source := textOf(fields["source"])
	tools := map[string]any{}

	for _, name := range generated[describedBy(fields)] {
		tools[name] = source + "." + name
	}

	for name := range objectOf(objectOf(fields["definition"])["tools"]) {
		tools[name] = source + "." + name
	}

	return tools
}

func described(fields map[string]any) any {
	kind := describedBy(fields)
	if kind == "" {
		return nil
	}

	return map[string]any{
		"kind":    kind,
		"hash":    strings.Repeat("a", 64),
		"readAt":  "2026-10-05T09:00:00.000Z",
		"problem": nil,
		"tools":   len(generated[kind]),
	}
}

func textOf(value any) string {
	text, _ := value.(string)

	return text
}

func objectOf(value any) map[string]any {
	object, _ := value.(map[string]any)

	return object
}

func listOf(value any) []any {
	listed, _ := value.([]any)

	return listed
}
