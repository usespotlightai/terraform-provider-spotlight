package provider

import (
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

var diagnosedLineFields = fieldpath.Locator{
	Schema: types.ObjectType{AttrTypes: map[string]attr.Type{
		"role": types.StringType,
		"station": types.ListType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{
			"passes_to": types.SetType{ElemType: types.StringType},
		}}},
	}},
	Renames: map[string]string{"stations": "station"},
}

func pathOf(d diag.Diagnostic) path.Path {
	if with, ok := d.(diag.DiagnosticWithPath); ok {
		return with.Path()
	}

	return path.Empty()
}

func TestEachFieldErrorLandsOnItsAttribute(t *testing.T) {
	var diags diag.Diagnostics

	addAPIError(&diags, "Could not create the line", &client.Problem{
		Status: 422, Type: client.ProblemInvalid, Title: "The line is not valid",
		Errors: []client.FieldError{
			{Path: "stations[1].passesTo", Message: "names nowhere", Severity: "error"},
			{Path: "role", Message: "claims the role delivery, which another line does", Severity: "error"},
			{Path: "", Message: "is not a line", Severity: "error"},
			{Path: "stations[0]", Message: "has no prompt", Severity: "warning"},
		},
	}, diagnosedLineFields, client.ScopeConfigure)

	errs := diags.Errors()
	if len(errs) != 3 {
		t.Fatalf("errors %v", errs)
	}

	if !pathOf(errs[0]).Equal(path.Root("station").AtListIndex(1).AtName("passes_to")) || !strings.HasPrefix(errs[0].Detail(), "station[1].passes_to: names nowhere") {
		t.Errorf("first %s: %s", pathOf(errs[0]), errs[0].Detail())
	}

	if !pathOf(errs[1]).Equal(path.Root("role")) {
		t.Errorf("second at %s", pathOf(errs[1]))
	}

	if len(pathOf(errs[2]).Steps()) != 0 {
		t.Errorf("a whole-resource error landed at %s", pathOf(errs[2]))
	}

	if warnings := diags.Warnings(); len(warnings) != 1 || !pathOf(warnings[0]).Equal(path.Root("station").AtListIndex(0)) {
		t.Errorf("warnings %v", warnings)
	}
}

func TestAStationAlertProblemLandsOnItsBlock(t *testing.T) {
	var diags diag.Diagnostics

	addAPIError(&diags, "Could not create the line", &client.Problem{
		Status: 422, Type: client.ProblemInvalid, Title: "The line is not valid",
		Errors: []client.FieldError{
			{Path: "stations[1].alerts[0].risesBy", Message: "must be above 0", Severity: "error"},
			{Path: "stations[1].judge.sample", Message: "is a share from 0 to 1", Severity: "error"},
		},
	}, lineFields, client.ScopeConfigure)

	errs := diags.Errors()
	if len(errs) != 2 {
		t.Fatalf("errors %v", errs)
	}

	if !pathOf(errs[0]).Equal(path.Root("station").AtListIndex(1).AtName("alert").AtListIndex(0).AtName("rises_by")) {
		t.Errorf("alert at %s", pathOf(errs[0]))
	}

	if !pathOf(errs[1]).Equal(path.Root("station").AtListIndex(1).AtName("judge").AtName("sample")) {
		t.Errorf("judge at %s", pathOf(errs[1]))
	}
}

func TestAStationBlockProblemLandsOnTheBlock(t *testing.T) {
	var diags diag.Diagnostics

	addAPIError(&diags, "Could not create the line", &client.Problem{
		Status: 422, Type: client.ProblemInvalid, Title: "The line is not valid",
		Errors: []client.FieldError{
			{Path: "stations[1].alerts[0]", Message: "sets exactly one of risesBy, above, fallsBy, below", Severity: "error"},
			{Path: "stations[0].judge", Message: "is only for a station where work runs", Severity: "error"},
			{Path: "stations[2].alerts", Message: "are only for a station where work runs", Severity: "error"},
			{Path: "stations[1].alerts[1].fallsBy", Message: "falls by a share above 0 and at most 1", Severity: "error"},
		},
	}, lineFields, client.ScopeConfigure)

	errs := diags.Errors()
	if len(errs) != 4 {
		t.Fatalf("errors %v", errs)
	}

	for i, want := range []path.Path{
		path.Root("station").AtListIndex(1).AtName("alert").AtListIndex(0),
		path.Root("station").AtListIndex(0).AtName("judge"),
		path.Root("station").AtListIndex(2).AtName("alert"),
		path.Root("station").AtListIndex(1).AtName("alert").AtListIndex(1).AtName("falls_by"),
	} {
		if !pathOf(errs[i]).Equal(want) {
			t.Errorf("error %d at %s, want %s", i, pathOf(errs[i]), want)
		}
	}
}

func TestALineReplayCheckOrSourceProblemLandsOnItsAttribute(t *testing.T) {
	var diags diag.Diagnostics

	addAPIError(&diags, "Could not create the line", &client.Problem{
		Status: 422, Type: client.ProblemInvalid, Title: "The line is not valid",
		Errors: []client.FieldError{
			{Path: "replayCheck.maxRegressions", Message: "is how many tasks a planned change may regress on and still pass, a whole number from 0", Severity: "error"},
			{Path: "replayCheck", Message: "is not valid", Severity: "error"},
			{Path: "source.path", Message: "is a Terraform file, ending in .tf", Severity: "error"},
			{Path: "source.repository", Message: "is the GitHub repository that contains the line’s Terraform, as owner/name: is named owner/name", Severity: "error"},
		},
	}, lineFields, client.ScopeConfigure)

	errs := diags.Errors()
	if len(errs) != 4 {
		t.Fatalf("errors %v", errs)
	}

	for i, want := range []path.Path{
		path.Root("replay_check").AtName("max_regressions"),
		path.Root("replay_check"),
		path.Root("source").AtName("path"),
		path.Root("source").AtName("repository"),
	} {
		if !pathOf(errs[i]).Equal(want) {
			t.Errorf("error %d at %s, want %s", i, pathOf(errs[i]), want)
		}
	}
}

func TestAConflictSaysWhatToDo(t *testing.T) {
	var diags diag.Diagnostics

	version := int64(4)
	addAPIError(&diags, "Could not update the line", &client.Problem{
		Status: 409, Type: client.ProblemVersionConflict, Title: "The line changed since it was read",
		Detail: "The line is at version 4", CurrentVersion: &version,
	}, diagnosedLineFields, client.ScopeConfigure)

	if !diags.HasError() || !strings.Contains(diags.Errors()[0].Detail(), "Run terraform plan again") {
		t.Errorf("diags %v", diags)
	}
}

func TestAnyOtherFailureIsReportedAsIs(t *testing.T) {
	var diags diag.Diagnostics

	addAPIError(&diags, "Could not read the line", errors.New("connection refused"), diagnosedLineFields, client.ScopeRead)

	if diags.Errors()[0].Detail() != "connection refused" {
		t.Errorf("diags %v", diags)
	}
}

func TestSplitImportID(t *testing.T) {
	workspace, address, ok := splitImportID("ws-1/core/delivery")
	if !ok || workspace != "ws-1" || address != "core/delivery" {
		t.Errorf("got %q %q %v", workspace, address, ok)
	}

	for _, bad := range []string{"core", "/core", "ws-1/", ""} {
		if _, _, ok := splitImportID(bad); ok {
			t.Errorf("SplitImportID(%q) should fail", bad)
		}
	}
}

func TestAForbiddenCallNamesTheScopeTheTokenLacks(t *testing.T) {
	changing := &client.Problem{
		Status: 403, Type: client.ProblemForbidden, Title: "Not allowed to change the secret",
		Detail: "You can read this secret but not change it. Your workspace role or token scopes do not allow it.",
	}

	for needs, says := range map[client.Scope]string{
		client.ScopeConfigure: "lacks the configure scope",
		client.ScopeSecrets:   "lacks the secrets scope",
		client.ScopeRunners:   "lacks the runners scope",
	} {
		var diags diag.Diagnostics

		addAPIError(&diags, "Could not update the secret", changing, diagnosedLineFields, needs)

		detail := diags.Errors()[0].Detail()
		if !strings.HasPrefix(detail, "You can read this secret") || !strings.Contains(detail, says) || !strings.Contains(detail, "configure, secrets, runners and trusts") {
			t.Errorf("%s: detail %q", needs, detail)
		}
	}
}

func TestAForbiddenPlanNamesThePlanScope(t *testing.T) {
	var diags diag.Diagnostics

	addAPIError(&diags, "Could not check", &client.Problem{
		Status: 403, Type: client.ProblemForbidden, Title: "Not allowed to plan",
	}, diagnosedLineFields, client.ScopeConfigure)

	if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "lacks the plan scope") {
		t.Errorf("detail %q", detail)
	}
}

func forbiddenUpdateDetail(problem *client.Problem, needs client.Scope) string {
	var diags diag.Diagnostics

	addAPIError(&diags, "Could not update", problem, diagnosedLineFields, needs)

	return diags.Errors()[0].Detail()
}

func TestTheNamedPermissionOutranksTheResourceKind(t *testing.T) {
	for permission, says := range map[client.Permission]string{
		client.PermissionRead:             "lacks the read scope",
		client.PermissionPlan:             "lacks the plan scope",
		client.PermissionConfigure:        "lacks the configure scope",
		client.PermissionSecrets:          "lacks the secrets scope",
		client.PermissionRunners:          "lacks the runners scope",
		client.PermissionTrusts:           "lacks the trusts scope",
		client.PermissionAdminister:       "a token never administers",
		client.PermissionCreateWorkspaces: "cannot create one",
		"a-permission-from-a-newer-api":   "do not allow this",
	} {
		detail := forbiddenUpdateDetail(&client.Problem{
			Status: 403, Type: client.ProblemForbidden, Title: "Not allowed to change the line", Permission: permission,
		}, client.ScopeRunners)

		if !strings.Contains(detail, says) || (permission != client.PermissionRunners && strings.Contains(detail, "lacks the runners scope")) {
			t.Errorf("%s: detail %q", permission, detail)
		}
	}
}

func TestWithoutAPermissionTheTitleThenTheKindSays(t *testing.T) {
	for title, says := range map[string]string{
		"Not allowed to administer the workspace": "a token never administers",
		"Not allowed to create a workspace":       "cannot create one",
		"Not allowed to change the runner-pool":   "lacks the runners scope",
	} {
		if detail := forbiddenUpdateDetail(&client.Problem{Status: 403, Type: client.ProblemForbidden, Title: title}, client.ScopeRunners); !strings.Contains(detail, says) {
			t.Errorf("%s: detail %q", title, detail)
		}
	}
}
