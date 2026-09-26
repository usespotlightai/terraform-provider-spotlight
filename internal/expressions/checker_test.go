package expressions_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/dynamic"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/expressions"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

type asked struct {
	position   string
	expression string
}

type stub struct {
	listed   int
	asked    []asked
	answer   func(position, expression string) (client.ExpressionCheck, error)
	listFail error
}

func (s *stub) ExpressionPositions(context.Context) ([]client.Position, error) {
	s.listed++
	if s.listFail != nil {
		return nil, s.listFail
	}

	return []client.Position{
		{Position: "integration.webhook.events[*].when", Returns: "bool"},
		{Position: "integration.actions[*].request.body"},
		{Position: "integration.on[*].to[*]"},
		{Position: "hook.when", Returns: "bool"},
		{Position: "hook.steps[*].with[*]"},
		{Position: "hook.steps[*].request.body"},
	}, nil
}

func (s *stub) CheckExpression(_ context.Context, position, expression string, _ client.Definition) (client.ExpressionCheck, error) {
	s.asked = append(s.asked, asked{position, expression})
	if s.answer == nil {
		return client.ExpressionCheck{Type: "bool"}, nil
	}

	return s.answer(position, expression)
}

var definition = dynamic.FromJSON(map[string]any{
	"integration": "linear",
	"webhook": map[string]any{
		"events": []any{
			map[string]any{"when": `${{ body.type == "Issue" }}`, "raise": map[string]any{"happening": "linear.issue.completed"}},
		},
	},
	"actions": map[string]any{
		"comment": map[string]any{"request": map[string]any{
			"url":  "https://api.linear.app/graphql",
			"body": map[string]any{"query": "mutation { ${{ inputs.body }} and ${{ inputs.title }} }"},
		}},
	},
	"on": map[string]any{
		"task.opened": map[string]any{"do": "comment", "to": map[string]any{"thread": "${{ ask.body }}"}},
	},
})

func TestItChecksEachExpressionWhereItIsWritten(t *testing.T) {
	api := &stub{}

	diags := expressions.NewChecker(api).Check(context.Background(), definition)
	if diags.HasError() {
		t.Fatal(diags)
	}

	want := []asked{
		{"integration.actions.comment.request.body.query", "inputs.body"},
		{"integration.actions.comment.request.body.query", "inputs.title"},
		{`integration.on["task.opened"].to.thread`, "ask.body"},
		{"integration.webhook.events[0].when", `body.type == "Issue"`},
	}

	if !reflect.DeepEqual(api.asked, want) {
		t.Errorf("asked %v\nwant  %v", api.asked, want)
	}
}

func TestItReadsThePositionsOnce(t *testing.T) {
	api := &stub{}
	checker := expressions.NewChecker(api)

	checker.Check(context.Background(), definition)
	checker.Check(context.Background(), definition)

	if api.listed != 1 {
		t.Errorf("listed the positions %d times", api.listed)
	}
}

func TestItAttachesErrorsToTheAttribute(t *testing.T) {
	api := &stub{answer: func(position, _ string) (client.ExpressionCheck, error) {
		if !strings.HasSuffix(position, ".thread") {
			return client.ExpressionCheck{Type: "bool"}, nil
		}

		return client.ExpressionCheck{
			Errors: []client.ExpressionError{{Message: "No such key: bdoy", Span: &client.Span{Start: 4, End: 8}, Suggestion: "ask.body"}},
			Warnings: []client.ExpressionWarning{
				{Position: "integration.webhook.events[0].lookup.capture.body", Message: "shadows the built-in body"},
			},
		}, nil
	}}

	diags := expressions.NewChecker(api).Check(context.Background(), definition)

	errs := diags.Errors()
	if len(errs) != 1 {
		t.Fatalf("errors %v", errs)
	}

	wantPath := path.Root("definition").AtName("on").AtName("task.opened").AtName("to").AtName("thread")
	if got := pathOf(errs[0]); !got.Equal(wantPath) {
		t.Errorf("error at %s, want %s", got, wantPath)
	}

	if errs[0].Summary() != `Invalid expression in definition.on["task.opened"].to.thread` {
		t.Errorf("summary %q", errs[0].Summary())
	}

	if detail := errs[0].Detail(); !strings.Contains(detail, "No such key: bdoy") || !strings.Contains(detail, "    ^^^^") || !strings.Contains(detail, "Did you mean ask.body?") {
		t.Errorf("detail %q", detail)
	}

	warnings := diags.Warnings()
	if len(warnings) != 1 || !pathOf(warnings[0]).Equal(path.Root("definition").AtName("webhook").AtName("events").AtListIndex(0).AtName("lookup").AtName("capture").AtName("body")) {
		t.Errorf("warnings %v", warnings)
	}
}

func TestItSkipsWhatIsNotKnown(t *testing.T) {
	api := &stub{}

	partly := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{"integration": types.StringType, "on": types.ObjectType{AttrTypes: map[string]attr.Type{
			"task.opened": types.ObjectType{AttrTypes: map[string]attr.Type{"to": types.ObjectType{AttrTypes: map[string]attr.Type{
				"thread": types.StringType, "channel": types.StringType,
			}}}},
		}}},
		map[string]attr.Value{
			"integration": types.StringValue("slack"),
			"on": types.ObjectValueMust(map[string]attr.Type{"task.opened": types.ObjectType{AttrTypes: map[string]attr.Type{"to": types.ObjectType{AttrTypes: map[string]attr.Type{"thread": types.StringType, "channel": types.StringType}}}}},
				map[string]attr.Value{"task.opened": types.ObjectValueMust(
					map[string]attr.Type{"to": types.ObjectType{AttrTypes: map[string]attr.Type{"thread": types.StringType, "channel": types.StringType}}},
					map[string]attr.Value{"to": types.ObjectValueMust(
						map[string]attr.Type{"thread": types.StringType, "channel": types.StringType},
						map[string]attr.Value{"thread": types.StringUnknown(), "channel": types.StringValue("${{ ask.channel }}")},
					)},
				)}),
		},
	))

	expressions.NewChecker(api).Check(context.Background(), partly)

	if len(api.asked) != 1 || api.asked[0].expression != "ask.channel" {
		t.Errorf("asked %v", api.asked)
	}

	api.asked = nil
	expressions.NewChecker(api).Check(context.Background(), types.DynamicUnknown())
	expressions.NewChecker(api).Check(context.Background(), types.DynamicNull())

	if len(api.asked) != 0 {
		t.Errorf("asked %v about a definition not known", api.asked)
	}
}

func TestAnInvalidDefinitionIsReportedOnceAtItsAttribute(t *testing.T) {
	api := &stub{answer: func(string, string) (client.ExpressionCheck, error) {
		return client.ExpressionCheck{}, &client.Problem{
			Status: 422, Type: client.ProblemInvalid, Title: "The request is not valid",
			Errors: []client.FieldError{{Path: "definition.types.Issue.id", Message: "is a type, as in string or list<Issue>"}},
		}
	}}

	diags := expressions.NewChecker(api).Check(context.Background(), definition)

	errs := diags.Errors()
	if len(errs) != 1 || !pathOf(errs[0]).Equal(path.Root("definition").AtName("types").AtName("Issue").AtName("id")) {
		t.Errorf("errors %v", errs)
	}

	if len(api.asked) != 1 {
		t.Errorf("kept asking after the definition was refused: %v", api.asked)
	}
}

func TestAnUnreachableControlPlaneOnlyWarns(t *testing.T) {
	api := &stub{listFail: errors.New("connection refused")}

	diags := expressions.NewChecker(api).Check(context.Background(), definition)

	if diags.HasError() || len(diags.Warnings()) != 1 {
		t.Errorf("diags %v", diags)
	}
}

func TestPieces(t *testing.T) {
	cases := map[string][]string{
		"${{ body.type }}":                       {"body.type"},
		"  ${{ a }}  ":                           {"a"},
		"Bearer ${{ secret.TOKEN }}":             {"secret.TOKEN"},
		"${{ a }}-${{ b }}x":                     {"a", "b"},
		"https://api.linear.app/graphql":         nil,
		"${{\n  body.items.map(i, i.id)\n}}":     {"body.items.map(i, i.id)"},
		"a ${{ first }} and ${{ second }} and z": {"first", "second"},
	}

	for text, want := range cases {
		if got := expressions.Pieces(text); !reflect.DeepEqual(got, want) {
			t.Errorf("Pieces(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestItChecksAHookWhereEachExpressionIsWritten(t *testing.T) {
	api := &stub{answer: func(position, _ string) (client.ExpressionCheck, error) {
		if position != "hook.steps[0].with.text" {
			return client.ExpressionCheck{Type: "bool"}, nil
		}

		return client.ExpressionCheck{Errors: []client.ExpressionError{{Message: "No such key: qeury", Span: &client.Span{Start: 13, End: 18}, Suggestion: "event.inputs.query"}}}, nil
	}}

	steps := dynamic.FromJSON([]any{
		map[string]any{"name": "notify", "call": "slack.post-message", "with": map[string]any{"text": "Searched ${{ event.inputs.qeury }}"}},
		map[string]any{"name": "audit", "request": map[string]any{"url": "https://audit.acme.dev/${{ task.id }}", "body": map[string]any{"q": "${{ event.inputs }}"}}},
	})

	subject := expressions.Subject{
		Resource:   "hook",
		Fields:     fieldpath.Locator{},
		Definition: client.Definition{"key": "grafana-reads"},
		Complete:   true,
		Written: append(
			[]expressions.Written{{At: fieldpath.Path{fieldpath.Key("when")}, Expressions: []string{"event.inputs.query != ''"}}},
			expressions.TemplatesIn(steps, fieldpath.Path{fieldpath.Key("steps")})...,
		),
	}

	diags := expressions.NewChecker(api).CheckSubject(context.Background(), subject)

	want := []asked{
		{"hook.when", "event.inputs.query != ''"},
		{"hook.steps[0].with.text", "event.inputs.qeury"},
		{"hook.steps[1].request.body.q", "event.inputs"},
	}
	if !reflect.DeepEqual(api.asked, want) {
		t.Errorf("asked %v\nwant  %v", api.asked, want)
	}

	errs := diags.Errors()
	if len(errs) != 1 || !pathOf(errs[0]).Equal(path.Root("steps").AtListIndex(0).AtName("with").AtName("text")) {
		t.Fatalf("errors %v", errs)
	}

	if detail := errs[0].Detail(); !strings.Contains(detail, "Did you mean event.inputs.query?") {
		t.Errorf("detail %q", detail)
	}
}

func pathOf(d diag.Diagnostic) path.Path {
	if with, ok := d.(diag.DiagnosticWithPath); ok {
		return with.Path()
	}

	return path.Empty()
}
