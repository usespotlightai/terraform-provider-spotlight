package resourcekit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

var line = fieldpath.Locator{
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

	resourcekit.Failed(&diags, "Could not create the line", &spotlight.Problem{
		Status: 422, Type: spotlight.ProblemInvalid, Title: "The line is not valid",
		Errors: []spotlight.FieldError{
			{Path: "stations[1].passesTo", Message: "names nowhere", Severity: "error"},
			{Path: "role", Message: "claims the role delivery, which another line does", Severity: "error"},
			{Path: "", Message: "is not a line", Severity: "error"},
			{Path: "stations[0]", Message: "has no prompt", Severity: "warning"},
		},
	}, line)

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

func TestAConflictSaysWhatToDo(t *testing.T) {
	var diags diag.Diagnostics

	version := int64(4)
	resourcekit.Failed(&diags, "Could not update the line", &spotlight.Problem{
		Status: 409, Type: spotlight.ProblemMovedOn, Title: "The line changed since it was read",
		Detail: "The line is at version 4", CurrentVersion: &version,
	}, line)

	if !diags.HasError() || !strings.Contains(diags.Errors()[0].Detail(), "Run terraform plan again") {
		t.Errorf("diags %v", diags)
	}
}

func TestAnyOtherFailureIsReportedAsIs(t *testing.T) {
	var diags diag.Diagnostics

	resourcekit.Failed(&diags, "Could not read the line", errors.New("connection refused"), line)

	if diags.Errors()[0].Detail() != "connection refused" {
		t.Errorf("diags %v", diags)
	}
}

func TestSplitImportID(t *testing.T) {
	workspace, address, ok := resourcekit.SplitImportID("ws-1/core/delivery")
	if !ok || workspace != "ws-1" || address != "core/delivery" {
		t.Errorf("got %q %q %v", workspace, address, ok)
	}

	for _, bad := range []string{"core", "/core", "ws-1/", ""} {
		if _, _, ok := resourcekit.SplitImportID(bad); ok {
			t.Errorf("SplitImportID(%q) should fail", bad)
		}
	}
}
