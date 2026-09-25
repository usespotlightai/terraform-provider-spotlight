package fieldpath_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
)

func TestParseReadsWhatTheAPIWrites(t *testing.T) {
	for _, written := range []string{
		"stations[2].passesTo",
		`on["task.opened"].to.thread`,
		"integration.webhook.events[*].when",
		`files["SKILL.md"]`,
		`headers.Content-Type`,
		`capture["0"]`,
		"",
	} {
		parsed, err := fieldpath.Parse(written)
		if err != nil {
			t.Fatalf("Parse(%q): %v", written, err)
		}

		if got := parsed.String(); got != written {
			t.Errorf("Parse(%q).String() = %q", written, got)
		}
	}
}

func TestParseRefusesWhatIsNotAPath(t *testing.T) {
	for _, written := range []string{"[0]", "a..b", `a["unterminated]`, "a[x]"} {
		if _, err := fieldpath.Parse(written); err == nil {
			t.Errorf("Parse(%q) should fail", written)
		}
	}
}

func TestWithinMatchesPatternsAndWhatIsInsideThem(t *testing.T) {
	pattern, _ := fieldpath.Parse("actions[*].request.body")

	cases := map[string]bool{
		"actions.comment.request.body":       true,
		"actions.comment.request.body.query": true,
		`actions["a.b"].request.body[0]`:     true,
		"actions.comment.request.url":        false,
		"actions.comment.request":            false,
	}

	for written, want := range cases {
		at, err := fieldpath.Parse(written)
		if err != nil {
			t.Fatal(err)
		}

		if got := at.Within(pattern); got != want {
			t.Errorf("%s within %s = %v, want %v", written, pattern, got, want)
		}
	}
}

func TestSnakeCase(t *testing.T) {
	for from, want := range map[string]string{"passesTo": "passes_to", "teamId": "team_id", "key": "key"} {
		if got := fieldpath.SnakeCase(from); got != want {
			t.Errorf("SnakeCase(%q) = %q, want %q", from, got, want)
		}
	}
}

var lineSchema = types.ObjectType{AttrTypes: map[string]attr.Type{
	"name":       types.StringType,
	"repository": types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"setup": types.StringType}}},
	"station": types.ListType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{
		"key":       types.StringType,
		"passes_to": types.SetType{ElemType: types.StringType},
		"tools":     types.ListType{ElemType: types.StringType},
	}}},
	"reaction": types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"to": types.StringType}}},
	"reviews": types.ObjectType{AttrTypes: map[string]attr.Type{
		"skip": types.ObjectType{AttrTypes: map[string]attr.Type{"authors": types.ListType{ElemType: types.StringType}}},
	}},
	"files":      types.MapType{ElemType: types.StringType},
	"definition": types.DynamicType,
}}

func TestLocateFindsTheTerraformAttribute(t *testing.T) {
	at := fieldpath.Locator{Schema: lineSchema, Renames: map[string]string{"repositories": "repository", "stations": "station", "reactions": "reaction"}}

	cases := []struct {
		api     string
		path    path.Path
		written string
	}{
		{"stations[2].passesTo", path.Root("station").AtListIndex(2).AtName("passes_to"), "station[2].passes_to"},
		{"stations[2].passesTo[1]", path.Root("station").AtListIndex(2).AtName("passes_to"), "station[2].passes_to"},
		{"stations[0].tools[1]", path.Root("station").AtListIndex(0).AtName("tools").AtListIndex(1), "station[0].tools[1]"},
		{"reactions[3].to", path.Root("reaction"), "reaction"},
		{"repositories[0].setup", path.Root("repository"), "repository"},
		{"reviews.skip.authors[0]", path.Root("reviews").AtName("skip").AtName("authors").AtListIndex(0), "reviews.skip.authors[0]"},
		{`files["SKILL.md"]`, path.Root("files").AtMapKey("SKILL.md"), `files["SKILL.md"]`},
		{`definition.on["task.opened"].to.thread`, path.Root("definition").AtName("on").AtName("task.opened").AtName("to").AtName("thread"), `definition.on["task.opened"].to.thread`},
		{"definition.webhook.events[0].when", path.Root("definition").AtName("webhook").AtName("events").AtListIndex(0).AtName("when"), "definition.webhook.events[0].when"},
		{"nowhere.at.all", path.Empty(), ""},
		{"", path.Empty(), ""},
		{"not a path", path.Empty(), ""},
	}

	for _, each := range cases {
		located := at.Locate(each.api)

		if !located.Path.Equal(each.path) {
			t.Errorf("Locate(%q).Path = %s, want %s", each.api, located.Path, each.path)
		}

		if located.Written != each.written {
			t.Errorf("Locate(%q).Written = %q, want %q", each.api, located.Written, each.written)
		}
	}
}

func TestLocateUnderPutsAWholeBodyInOneAttribute(t *testing.T) {
	at := fieldpath.Locator{Schema: lineSchema, Under: fieldpath.Path{fieldpath.Key("definition")}}

	located := at.Locate("actions.comment.request.url")

	want := path.Root("definition").AtName("actions").AtName("comment").AtName("request").AtName("url")
	if !located.Path.Equal(want) || located.Written != "definition.actions.comment.request.url" {
		t.Errorf("got %s (%q)", located.Path, located.Written)
	}

	if root := at.Locate(""); root.Written != "definition" {
		t.Errorf("the whole body is the attribute, got %q", root.Written)
	}
}
