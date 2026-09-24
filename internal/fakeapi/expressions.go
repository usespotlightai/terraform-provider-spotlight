package fakeapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

// Positions are the positions the fake says expressions are written at.
var Positions = []string{
	"integration.webhook.events[*].when",
	"integration.webhook.events[*].raise.marks[*].value",
	"integration.actions[*].request.url",
	"integration.actions[*].request.headers[*]",
	"integration.actions[*].request.body",
	"integration.on[*].to[*]",
}

var declared = map[string][]string{
	"body":   {"type", "data", "action"},
	"ask":    {"body", "author"},
	"inputs": {"body", "title"},
	"task":   {"marks", "title"},
}

var reference = regexp.MustCompile(`\b([a-z][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)`)

func (s *Server) positions(w http.ResponseWriter, _ *http.Request) {
	listed := make([]any, 0, len(Positions))

	for _, position := range Positions {
		var returns any
		if strings.HasSuffix(position, ".when") {
			returns = "bool"
		}

		listed = append(listed, map[string]any{"position": position, "returns": returns, "sees": []any{}, "alsoSees": []any{}})
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

	if source, _ := body.Definition["integration"].(string); source == "" {
		problem(w, http.StatusUnprocessableEntity, "invalid", "The request is not valid", "", []fieldError{{"definition.integration", "names the host this integration defines, as in linear"}})
		return
	}

	at, err := fieldpath.Parse(body.Position)
	if err != nil || len(at) < 2 || at[0].Key != "integration" || !holdsText(body.Definition, at[1:]) {
		problem(w, http.StatusUnprocessableEntity, "invalid", "The request is not valid", "", []fieldError{{"position", "holds no expression in the definition, so nothing is written there to check"}})
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

func holdsText(held any, at fieldpath.Path) bool {
	for _, step := range at {
		switch typed := held.(type) {
		case map[string]any:
			if step.IsIndex() {
				return false
			}
			held = typed[step.Key]
		case []any:
			if !step.IsIndex() || step.Index >= len(typed) {
				return false
			}
			held = typed[step.Index]
		default:
			return false
		}
	}

	_, ok := held.(string)

	return ok
}

func contains(held []string, want string) bool {
	for _, each := range held {
		if each == want {
			return true
		}
	}

	return false
}

func nearest(held []string, want string) string {
	best, score := held[0], -1

	for _, each := range held {
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
