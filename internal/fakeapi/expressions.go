package fakeapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

// Positions are the positions that can contain expressions in the fake.
var Positions = []string{
	"integration.webhook.events[*].when",
	"integration.webhook.events[*].raise.marks[*].value",
	"integration.actions[*].request.url",
	"integration.actions[*].request.headers[*]",
	"integration.actions[*].request.body",
	"integration.on[*].to[*]",
	"hook.when",
	"hook.steps[*].with[*]",
	"hook.steps[*].request.url",
	"hook.steps[*].request.headers[*]",
	"hook.steps[*].request.body",
}

var declared = map[string][]string{
	"body":   {"type", "data", "action"},
	"ask":    {"body", "author"},
	"inputs": {"body", "title"},
	"task":   {"marks", "title"},
	"event":  {"inputs", "output", "tool", "title", "changed", "failed"},
}

var reference = regexp.MustCompile(`\b([a-z][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)`)

func (s *Server) positions(w http.ResponseWriter, _ *http.Request) {
	listed := make([]any, 0, len(Positions))

	for _, position := range Positions {
		var returns any
		if strings.HasSuffix(position, ".when") {
			returns = "bool"
		}

		listed = append(listed, map[string]any{"position": position, "returns": returns, "variables": []any{}, "addedVariables": []any{}})
	}

	writeJSON(w, http.StatusOK, "application/json", listed)
}

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Position   string         `json:"position"`
		Expression string         `json:"expression"`
		Definition map[string]any `json:"definition"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem(w, http.StatusUnprocessableEntity, "invalid", "The request is not valid", err.Error(), nil)
		return
	}

	s.mu.Lock()
	mayPlan := s.scopes["plan"] || s.scopes["configure"]
	s.mu.Unlock()

	if !mayPlan {
		problem(w, http.StatusForbidden, "forbidden", "Not allowed to plan", "Checking an expression is planning, which this token’s scopes do not allow", nil)
		return
	}

	at, err := fieldpath.Parse(body.Position)
	if err != nil || len(at) < 2 || !slices.Contains([]string{"integration", "hook"}, at[0].Key) {
		problem(w, http.StatusUnprocessableEntity, "invalid", "The request is not valid", "", []fieldError{{"position", "names an attribute of an integration or a hook, as in integration.webhook.events[0].when or hook.when"}})
		return
	}

	if problems := definitionProblems(at[0].Key, body.Definition); problems != nil {
		problem(w, http.StatusUnprocessableEntity, "invalid", "The request is not valid", "", problems)
		return
	}

	if !containsText(body.Definition, at[1:]) {
		problem(w, http.StatusUnprocessableEntity, "invalid", "The request is not valid", "", []fieldError{{"position", "contains no expression in the definition. Nothing is written there to check"}})
		return
	}

	errs := []any{}

	if strings.HasSuffix(body.Position, ".when") && strings.HasPrefix(strings.TrimSpace(body.Expression), `"`) {
		errs = append(errs, map[string]any{
			"message":    "must return a bool, not a string",
			"span":       map[string]any{"start": 0, "end": len(body.Expression)},
			"suggestion": nil,
		})
	}

	for _, found := range reference.FindAllStringSubmatchIndex(body.Expression, -1) {
		root, field := body.Expression[found[2]:found[3]], body.Expression[found[4]:found[5]]

		fields, known := declared[root]
		if !known || contains(fields, field) {
			continue
		}

		errs = append(errs, map[string]any{
			"message":    "No such key: " + field,
			"span":       map[string]any{"start": found[4], "end": found[5]},
			"suggestion": root + "." + nearest(fields, field),
		})
	}

	writeJSON(w, http.StatusOK, "application/json", map[string]any{"type": "bool", "errors": errs, "warnings": []any{}})
}

func definitionProblems(resource string, definition map[string]any) []fieldError {
	if resource == "integration" {
		if named, _ := definition["integration"].(string); named == "" {
			return []fieldError{{"definition.integration", "is the name of the integration, as in linear"}}
		}

		return nil
	}

	_, errs := hook(definition)
	for at, each := range errs {
		errs[at].path = "definition." + each.path
	}

	return errs
}

func containsText(value any, at fieldpath.Path) bool {
	for _, step := range at {
		switch typed := value.(type) {
		case map[string]any:
			if step.IsIndex() {
				return false
			}
			value = typed[step.Key]
		case []any:
			if !step.IsIndex() || step.Index >= len(typed) {
				return false
			}
			value = typed[step.Index]
		default:
			return false
		}
	}

	_, ok := value.(string)

	return ok
}

func contains(values []string, want string) bool {
	for _, each := range values {
		if each == want {
			return true
		}
	}

	return false
}

func nearest(values []string, want string) string {
	best, score := values[0], -1

	for _, each := range values {
		shared := 0
		for _, r := range want {
			if strings.ContainsRune(each, r) {
				shared++
			}
		}

		if shared > score {
			best, score = each, shared
		}
	}

	return best
}
