package fakeapi

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

var (
	hookEvents = []string{
		"tool.using", "tool.used", "run.starting", "run.ended", "handover.giving",
		"pull-request.opening", "pull-request.merged", "task.opened", "task.moved",
	}
	hookMatches = []string{"tools", "lines", "stations", "repositories", "paths"}
	toolName    = regexp.MustCompile(`^[a-z0-9]+\.[a-z0-9]+(-[a-z0-9]+)*$`)
	varName     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func hook(body map[string]any) (map[string]any, []fieldError) {
	var errs []fieldError

	key := strings.TrimSpace(textOf(body["key"]))
	if why := wordsLike("migrations-need-a-dba")(key); why != "" {
		errs = append(errs, fieldError{"key", why})
	}

	on := textOf(body["on"])
	if !slices.Contains(hookEvents, on) {
		errs = append(errs, fieldError{"on", "is one of " + strings.Join(hookEvents, ", ")})
	}

	match := map[string]any{}
	for _, field := range hookMatches {
		match[field] = texts(objectOf(body["match"])[field])
	}
	for at, tool := range listOf(match["tools"]) {
		if !toolName.MatchString(textOf(tool)) {
			errs = append(errs, fieldError{fmt.Sprintf("match.tools[%d]", at), "names a tool as the integration and the tool, as in grafana.search"})
		}
	}

	vars := map[string]any{}
	for name, value := range objectOf(body["vars"]) {
		if !varName.MatchString(name) {
			errs = append(errs, fieldError{keyed("vars", name), "is a name CEL can read, as in migrations"})
		}
		vars[name] = value
	}

	fields := map[string]any{
		"key":            key,
		"on":             on,
		"match":          match,
		"vars":           vars,
		"steps":          hookSteps(listOf(body["steps"]), &errs),
		"signing":        hookSigning(body["signing"], &errs),
		"failure":        "continue",
		"timeoutSeconds": 10.0,
	}

	if fields["signing"] == nil && slices.ContainsFunc(listOf(fields["steps"]), func(step any) bool { return objectOf(step)["request"] != nil }) {
		errs = append(errs, fieldError{"signing", "is missing; a hook that makes requests signs them, with an HMAC secret or an OIDC token"})
	}

	if when, given := body["when"].(string); given {
		fields["when"] = when
	}

	if failure, given := body["failure"].(string); given {
		if failure != "continue" && failure != "block" {
			errs = append(errs, fieldError{"failure", "is continue or block"})
		}
		fields["failure"] = failure
	}

	if timeout, given := body["timeoutSeconds"].(float64); given {
		if timeout != float64(int64(timeout)) || timeout < 1 || timeout > 30 {
			errs = append(errs, fieldError{"timeoutSeconds", "is a whole number of seconds from 1 to 30"})
		}
		fields["timeoutSeconds"] = timeout
	}

	return fields, errs
}

func hookSteps(listed []any, errs *[]fieldError) []any {
	if len(listed) == 0 {
		*errs = append(*errs, fieldError{"steps", "is what the hook does, in order, and there has to be one"})
	}

	steps := make([]any, 0, len(listed))
	names := map[string]bool{}

	for index, each := range listed {
		step := maps.Clone(objectOf(each))
		at := fmt.Sprintf("steps[%d]", index)

		nulls(step, at, errs)
		nulls(objectOf(step["request"]), at+".request", errs)

		name := textOf(step["name"])
		if !words(name) {
			*errs = append(*errs, fieldError{at + ".name", "is lowercase words joined by dashes, as in notify"})
		}
		if names[name] {
			*errs = append(*errs, fieldError{at + ".name", fmt.Sprintf("names the step %s twice", name)})
		}
		names[name] = true

		_, calls := step["call"].(string)
		request, requests := step["request"].(map[string]any)

		switch {
		case calls == requests:
			*errs = append(*errs, fieldError{at, "names exactly one of call and request. A step either calls a tool or makes a request"})
		case calls:
			if !toolName.MatchString(textOf(step["call"])) {
				*errs = append(*errs, fieldError{at + ".call", "names a tool as the integration and the tool, as in slack.post-message"})
			}
			orDefault(step, "with", map[string]any{})
		default:
			if _, given := step["with"]; given {
				*errs = append(*errs, fieldError{at + ".with", "gives inputs to a tool. It goes only with call"})
			}
			step["request"] = hookRequest(request, at+".request", errs)
		}

		steps = append(steps, step)
	}

	return steps
}

func hookRequest(given map[string]any, at string, errs *[]fieldError) map[string]any {
	request := maps.Clone(given)

	if textOf(request["url"]) == "" {
		*errs = append(*errs, fieldError{at + ".url", "is the address to send the request to"})
	}

	orDefault(request, "headers", map[string]any{})
	orDefault(request, "from", "spotlight")

	if from := request["from"]; from != "spotlight" && from != "network" {
		*errs = append(*errs, fieldError{at + ".from", "is spotlight or network"})
	}

	return request
}

func hookSigning(given any, errs *[]fieldError) any {
	signing, ok := given.(map[string]any)
	if !ok {
		return nil
	}

	if oidc, ok := signing["oidc"].(map[string]any); ok {
		if len(signing) > 1 {
			*errs = append(*errs, fieldError{"signing", "signs with an HMAC secret or an OIDC token, not both"})
		}
		if textOf(oidc["audience"]) == "" {
			*errs = append(*errs, fieldError{"signing.oidc.audience", "is who the token is for, as the endpoint checks it"})
		}

		return map[string]any{"oidc": map[string]any{"audience": oidc["audience"]}}
	}

	version, ok := signing["secretVersion"].(float64)
	if !ok || version < 1 || version != float64(int64(version)) {
		*errs = append(*errs, fieldError{"signing.secretVersion", "is a whole number from 1, changed to rotate the secret"})
	}

	return map[string]any{"secretVersion": signing["secretVersion"]}
}

func hookSecret(held *resource, fields, body map[string]any) (string, []fieldError) {
	signing := objectOf(fields["signing"])
	if signing == nil || signing["oidc"] != nil {
		return "", nil
	}

	secret, given := objectOf(body["signing"])["secret"].(string)
	if given && !signingSecret(secret) {
		return "", []fieldError{{"signing.secret", "is whsec_ followed by 24 to 64 base64-encoded random bytes, as Standard Webhooks signing secrets are"}}
	}

	switch {
	case given:
		return secret, nil
	case held == nil || objectOf(held.fields["signing"])["secretVersion"] != signing["secretVersion"]:
		return "", []fieldError{{"signing.secret", "is given whenever signing.secretVersion is set or changes, since that is how it is rotated"}}
	default:
		return "", nil
	}
}

func nulls(held map[string]any, at string, errs *[]fieldError) {
	for _, name := range slices.Sorted(maps.Keys(held)) {
		if held[name] == nil {
			*errs = append(*errs, fieldError{at + "." + name, "is null; leave it out instead"})
		}
	}
}
