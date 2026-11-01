package fakeapi

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
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
		return workspace(body)
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
	case "oidc-trust":
		return oidcTrust(body)
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

func workspace(body map[string]any) (map[string]any, []fieldError) {
	out, errs := named(body, "slug", "name")

	hosts := []string{}
	for at, each := range texts(body["outboundHosts"]) {
		host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(textOf(each))), ".")
		if wrong := outboundHostWrong(host); wrong != "" {
			errs = append(errs, fieldError{fmt.Sprintf("outboundHosts[%d]", at), fmt.Sprintf("Allowed host %q %s.", host, wrong)})
			continue
		}
		hosts = append(hosts, host)
	}
	slices.Sort(hosts)

	listed := []any{}
	for _, each := range slices.Compact(hosts) {
		listed = append(listed, each)
	}
	out["outboundHosts"] = listed

	return out, errs
}

func outboundHostWrong(host string) string {
	switch {
	case host == "":
		return "is empty. Write a host such as api.example.com"
	case host == "*":
		return "permits every host. Leave the list empty to permit every public host"
	case strings.HasPrefix(host, "*."):
		if suffix := host[2:]; strings.Contains(suffix, "*") || !strings.Contains(suffix, ".") {
			return "is not a pattern such as *.example.com"
		}
	case strings.ContainsAny(host, "*/: "):
		return "is not a host such as api.example.com or a pattern such as *.example.com"
	}

	if !hostLetters.MatchString(host) {
		return "has a character a host name cannot have. Write an internationalised host in its xn-- form"
	}

	return ""
}

var hostLetters = regexp.MustCompile(`^[a-z0-9._*-]+$`)

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
				errs = append(errs, fieldError{fmt.Sprintf("stations[%d].passesTo", at), fmt.Sprintf("must pass to work stations, but %s does not", textOf(station["name"]))})
			}
		}

		placed(station, "runners", fmt.Sprintf("stations[%d].runners", at), &errs)
		judged(station, at, &errs)
		stationSearched(station, at, &errs)

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

	lineSearched(definition, &errs)

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

var (
	durationText    = regexp.MustCompile(`^(\d{1,6})([mhd])$`)
	durationMinutes = map[string]float64{"m": 1, "h": 60, "d": 24 * 60}
	searchDefaults  = map[string]any{
		"enabled":        true,
		"monthlyCeiling": float64(50),
		"currency":       "USD",
		"ceiling":        float64(10),
		"period":         "7d",
		"onAlert":        true,
		"cooldown":       "3d",
	}
)

func minutesOf(value any) (float64, bool) {
	written := durationText.FindStringSubmatch(textOf(value))
	if written == nil {
		return 0, false
	}

	amount, _ := strconv.ParseFloat(written[1], 64)

	return amount * durationMinutes[written[2]], true
}

func isDuration(value any) bool {
	minutes, ok := minutesOf(value)

	return ok && minutes >= 60 && minutes <= 365*24*60
}

func lineSearched(definition map[string]any, errs *[]fieldError) {
	written, set := definition["search"].(map[string]any)
	search := map[string]any{}
	for field, fallback := range searchDefaults {
		search[field] = fallback
	}
	maps.Copy(search, written)
	search["currency"] = strings.ToUpper(strings.TrimSpace(textOf(search["currency"])))

	monthly, _ := search["monthlyCeiling"].(float64)
	ceiling, _ := search["ceiling"].(float64)
	if monthly <= 0 {
		*errs = append(*errs, fieldError{"search.monthlyCeiling", "must be above 0"})
	}
	if ceiling <= 0 || ceiling > 1000 {
		*errs = append(*errs, fieldError{"search.ceiling", "must be above 0 and at most 1,000"})
	} else if ceiling > monthly {
		*errs = append(*errs, fieldError{"search.ceiling", fmt.Sprintf("must be at most the monthly ceiling, %g", monthly)})
	}
	for _, field := range []string{"period", "cooldown"} {
		if !isDuration(search[field]) {
			*errs = append(*errs, fieldError{"search." + field, "must be a whole number of minutes, hours or days from 1h to 365d, as in 90m, 12h or 7d"})
		}
	}
	if longer(search["cooldown"], search["period"]) {
		*errs = append(*errs, fieldError{"search.cooldown", fmt.Sprintf("must be at most the period, %s", textOf(search["period"]))})
	}

	for at, each := range listOf(definition["stations"]) {
		station := objectOf(each)
		override := objectOf(station["search"])
		if override == nil {
			continue
		}
		if stationCeiling, ok := override["ceiling"].(float64); ok && stationCeiling > monthly {
			*errs = append(*errs, fieldError{fmt.Sprintf("stations[%d].search.ceiling", at), fmt.Sprintf("Line %s station %s search ceiling must be at most the line's monthly ceiling, %g %s", definition["key"], station["key"], monthly, search["currency"])})
		}
		period, cooldown := search["period"], search["cooldown"]
		if each, ok := override["period"]; ok {
			period = each
		}
		if each, ok := override["cooldown"]; ok {
			cooldown = each
		}
		if longer(cooldown, period) {
			*errs = append(*errs, fieldError{fmt.Sprintf("stations[%d].search.cooldown", at), fmt.Sprintf("Line %s station %s search cooldown must be at most its period, %s", definition["key"], station["key"], textOf(period))})
		}
	}

	if set {
		definition["search"] = search
	}
}

func longer(one, other any) bool {
	oneMinutes, oneRead := minutesOf(one)
	otherMinutes, otherRead := minutesOf(other)

	return oneRead && otherRead && oneMinutes > otherMinutes
}

func stationSearched(station map[string]any, at int, errs *[]fieldError) {
	search, ok := station["search"].(map[string]any)
	if !ok {
		return
	}

	field := fmt.Sprintf("stations[%d].search", at)
	if textOf(station["kind"]) != "work" {
		*errs = append(*errs, fieldError{field, "has no runs to improve"})
	}
	if ceiling, ok := search["ceiling"].(float64); ok && (ceiling <= 0 || ceiling > 1000) {
		*errs = append(*errs, fieldError{field + ".ceiling", "must be above 0 and at most 1,000"})
	}
	for _, each := range []string{"period", "cooldown"} {
		if _, ok := search[each]; ok && !isDuration(search[each]) {
			*errs = append(*errs, fieldError{field + "." + each, "must be a whole number of minutes, hours or days from 1h to 365d, as in 90m, 12h or 7d"})
		}
	}
	for each, most := range map[string]float64{"iterations": 10, "repeats": 5} {
		if count, ok := search[each].(float64); ok && (count < 1 || count > most || count != float64(int64(count))) {
			*errs = append(*errs, fieldError{field + "." + each, fmt.Sprintf("must be a whole number from 1 to %g", most)})
		}
	}
	if objective, ok := search["objective"]; ok && objective != "success" && objective != "cost" {
		*errs = append(*errs, fieldError{field + ".objective", "must be one of success, cost"})
	}
	if agent, ok := search["agent"].(map[string]any); ok && textOf(agent["harness"]) == "" {
		*errs = append(*errs, fieldError{field + ".agent.harness", "must not be empty"})
	}
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
		orDefault(judge, "passThreshold", float64(1))
		if len(claims) < 1 || len(claims) > 20 {
			*errs = append(*errs, fieldError{field + ".claims", "is the checkable claims the judge answers yes or no, from 1 to 20"})
		}
		for each, claim := range claims {
			if text := textOf(claim); text == "" || len([]rune(text)) > 500 {
				*errs = append(*errs, fieldError{fmt.Sprintf("%s.claims[%d]", field, each), "is a checkable claim from 1 to 500 characters"})
			}
		}
	} else if _, ok := judge["passThreshold"]; ok {
		*errs = append(*errs, fieldError{field + ".passThreshold", "is only set with claims"})
	}

	if share, ok := judge["passThreshold"].(float64); ok && (share < 0 || share > 1) {
		*errs = append(*errs, fieldError{field + ".passThreshold", "is the share of claims a change must meet to pass, from 0 to 1"})
	}

	station["judge"] = judge
}

func repositories(definition map[string]any, errs *[]fieldError) {
	listed := listOf(definition["repositories"])
	worked := make([]map[string]any, 0, len(listed))
	var images []string
	seen := map[string]bool{}

	for at, each := range listed {
		input := objectOf(each)
		name := strings.TrimSpace(textOf(input["name"]))
		if owner, rest, ok := strings.Cut(name, "/"); !ok || owner == "" || rest == "" || strings.Contains(rest, "/") {
			*errs = append(*errs, fieldError{fmt.Sprintf("repositories[%d].name", at), "A repository is named owner/name"})
		}
		if seen[name] {
			*errs = append(*errs, fieldError{"repositories", fmt.Sprintf("The %s line works in repositories that A line works %s twice, and each is cloned into a directory of its own name", definition["key"], name)})
		}
		seen[name] = true

		repository := map[string]any{"name": name}
		if image := strings.TrimSpace(textOf(input["image"])); image != "" {
			if len(image) > 255 || !imageReference.MatchString(image) {
				*errs = append(*errs, fieldError{fmt.Sprintf("repositories[%d].image", at), "A runner image is named by a reference such as ghcr.io/acme/runner:main, in lowercase, with an optional tag or digest"})
			}
			if !slices.Contains(images, image) {
				images = append(images, image)
			}
			repository["image"] = image
		}
		if setup := textOf(input["setup"]); setup != "" {
			if command := strings.TrimSpace(setup); command == "" || len(command) > 65536 {
				*errs = append(*errs, fieldError{fmt.Sprintf("repositories[%d].setup", at), "Invalid repository setup: must be at most 65536 characters"})
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

	named, _ := definition["integration"].(string)
	if named == "" {
		return nil, []fieldError{{"integration", "is the name of the integration, as in linear"}}
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

	return map[string]any{"integration": named, "definition": definition}, nil
}

var generated = map[string][]string{
	"openapi": {"find-pets", "add-pet"},
	"graphql": {"viewer", "create-issue"},
	"mcp":     {"search-issues", "delete-issue"},
}

func specKindOf(fields map[string]any) string {
	definition := objectOf(fields["definition"])
	for _, kind := range slices.Sorted(maps.Keys(generated)) {
		if definition[kind] != nil {
			return kind
		}
	}

	return ""
}

func provided(fields map[string]any) map[string]any {
	named := textOf(fields["integration"])
	tools := map[string]any{}

	for _, name := range generated[specKindOf(fields)] {
		tools[name] = named + "." + name
	}

	for name := range objectOf(objectOf(fields["definition"])["tools"]) {
		tools[name] = named + "." + name
	}

	return tools
}

func spec(fields map[string]any) any {
	kind := specKindOf(fields)
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
