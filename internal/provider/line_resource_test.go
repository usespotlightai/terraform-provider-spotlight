package provider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

const lineAddress = "spotlight_line.core_delivery"

const lineExported = `
resource "spotlight_line" "core_delivery" {
  team_id      = spotlight_team.core.id
  key          = "delivery"
  name         = "Delivery"
  role         = "review"
  repositories = ["acme/api"]
  runners      = ["vpc-internal", "claude-code@2.1.270"]

  station {
    key       = "backlog"
    name      = "Backlog"
    kind      = "backlog"
    color     = "#a3a8b0"
    diffs     = false
    decisions = false
    tools     = []
    skills    = []
    passes_to = ["implementation"]
    ends      = false
  }

  station {
    key             = "implementation"
    name            = "Implementation"
    kind            = "work"
    color           = "#6d78d5"
    harness         = "claude-code"
    profile         = "careful"
    harness_version = "2.1.270"
    prompt          = "Implement the task.\n"
    check           = "npm test"
    diffs           = true
    decisions       = true
    tools           = ["linear.comment"]
    skills          = ["grafana"]
    runners         = ["gpu"]
    passes_to       = ["code-review"]
    ends            = false
  }

  station {
    key       = "code-review"
    name      = "Code review"
    kind      = "work"
    color     = "#56b98d"
    diffs     = false
    decisions = false
    tools     = []
    skills    = []
    passes_to = ["implementation"]
    ends      = true
  }

  station {
    key       = "completed"
    name      = "Completed"
    kind      = "completed"
    color     = "#3fa46a"
    diffs     = false
    decisions = false
    tools     = []
    skills    = []
    passes_to = []
    ends      = false
  }

  reaction {
    happening = "github.checks.failed"
    from      = "implementation"
    to        = "implementation"
  }

  reviews {
    start   = "opened"
    mention = "@spotlight"

    skip {
      spotlight = true
      drafts    = true
      authors   = ["dependabot[bot]", "renovate[bot]"]
    }
  }
}
`

const lineMinimal = `
resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"
  role    = "review"

  station {
    key       = "backlog"
    name      = "Backlog"
    kind      = "backlog"
    passes_to = ["implementation"]
  }

  station {
    key     = "implementation"
    name    = "Implementation"
    harness = "claude-code"
  }

  station {
    key  = "completed"
    name = "Completed"
    kind = "completed"
  }

  reviews {
    start = "opened"

    skip {
      authors = ["renovate[bot]", "dependabot[bot]", "renovate[bot]"]
    }
  }
}
`

const lineGrown = `
resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"
  role    = "review"
  runners = ["vpc-internal"]

  station {
    key       = "backlog"
    name      = "Backlog"
    kind      = "backlog"
    passes_to = ["implementation"]
  }

  station {
    key             = "implementation"
    name            = "Implementation"
    harness         = "claude-code"
    harness_version = "v2.1.270"
    passes_to       = ["code-review"]
  }

  station {
    key       = "code-review"
    name      = "Code review"
    runners   = ["gpu"]
    passes_to = ["implementation"]
    ends      = true
  }

  station {
    key  = "completed"
    name = "Completed"
    kind = "completed"
  }

  reviews {
    start = "opened"

    skip {
      authors = ["renovate[bot]", "dependabot[bot]", "renovate[bot]"]
    }
  }
}
`

func lineImportID(state *terraform.State) (string, error) {
	workspace := state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID

	return workspace + "/core/delivery", nil
}

func lineStation(at int, name string) tfjsonpath.Path {
	return tfjsonpath.New("station").AtSliceIndex(at).AtMapKey(name)
}

func TestLineExportedShapeRoundTrips(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "line", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + lineExported,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("address"), knownvalue.StringExact("core/delivery")),
					statecheck.ExpectKnownValue(lineAddress, lineStation(1, "passes_to"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("code-review")})),
					statecheck.ExpectKnownValue(lineAddress, lineStation(1, "profile"), knownvalue.StringExact("careful")),
					statecheck.ExpectKnownValue(lineAddress, lineStation(1, "harness_version"), knownvalue.StringExact("2.1.270")),
					statecheck.ExpectKnownValue(lineAddress, lineStation(1, "runners"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("gpu")})),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("runners"), knownvalue.SetSizeExact(2)),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("reaction"), knownvalue.SetSizeExact(1)),
				},
			},
			{
				Config:   acctest.Workspace + lineExported,
				PlanOnly: true,
			},
			{
				ResourceName:      lineAddress,
				ImportState:       true,
				ImportStateIdFunc: lineImportID,
				ImportStateVerify: true,
			},
		},
	})
}

func TestLineFillsWhatItLeavesOutWithoutDrift(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + lineMinimal,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "color"), knownvalue.StringExact("#a3a8b0")),
					statecheck.ExpectKnownValue(lineAddress, lineStation(1, "kind"), knownvalue.StringExact("work")),
					statecheck.ExpectKnownValue(lineAddress, lineStation(1, "diffs"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(lineAddress, lineStation(2, "tools"), knownvalue.ListSizeExact(0)),
					statecheck.ExpectKnownValue(lineAddress, lineStation(2, "runners"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("runners"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("reviews").AtMapKey("skip").AtMapKey("authors"), knownvalue.SetSizeExact(2)),
				},
			},
			{
				Config:   acctest.Workspace + lineMinimal,
				PlanOnly: true,
			},
			{
				Config: acctest.Workspace + lineGrown,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(lineAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, lineStation(2, "color"), knownvalue.StringExact("#6d78d5")),
					statecheck.ExpectKnownValue(lineAddress, lineStation(3, "color"), knownvalue.StringExact("#3fa46a")),
					statecheck.ExpectKnownValue(lineAddress, lineStation(1, "harness_version"), knownvalue.StringExact("v2.1.270")),
					statecheck.ExpectKnownValue(lineAddress, lineStation(2, "runners"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("gpu")})),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("version"), knownvalue.Int64Exact(2)),
				},
			},
			{
				Config:   acctest.Workspace + lineGrown,
				PlanOnly: true,
			},
		},
	})
}

func TestLineDriftIsPlannedBack(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + lineMinimal,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("line", "core/delivery"), func(fields map[string]any) {
						definition, ok := fields["definition"].(map[string]any)
						if !ok {
							t.Fatalf("the fake holds no definition: %v", fields)
						}
						definition["name"] = "Renamed in the web app"
					})
				},
				Config: acctest.Workspace + lineMinimal,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(lineAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("name"), knownvalue.StringExact("Delivery")),
				},
			},
			{
				PreConfig: func() { fake.Forget(fake.IDOf("line", "core/delivery")) },
				Config:    acctest.Workspace + lineMinimal,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(lineAddress, plancheck.ResourceActionCreate)},
				},
			},
		},
	})
}

func TestLineRefusalNamesTheStationAttribute(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + `
resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"
  role    = "review"

  station {
    key  = "backlog"
    name = "Backlog"
  }

  station {
    key       = "implementation"
    name      = "Implementation"
    passes_to = ["nowhere"]
  }
}
`,
				ExpectError: regexp.MustCompile(`(?s)station\[1\]\.passes_to: names nowhere, which is not a station`),
			},
		},
	})
}

func lineRefused(t *testing.T, stations string, want *regexp.Regexp) {
	t.Helper()

	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + `
resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"
` + stations + `
}
`,
				ExpectError: want,
			},
		},
	})
}

func TestLineRefusesAStationKeyRunsSayToHandOver(t *testing.T) {
	lineRefused(t, `
  station {
    key       = "backlog"
    name      = "Backlog"
    kind      = "backlog"
    passes_to = ["review"]
  }

  station {
    key  = "review"
    name = "Review"
  }
`, regexp.MustCompile(`(?s)station\[1\]\.key: cannot be review, which is reserved`))
}

func TestLineHandsOnOnlyToStationsWorkIsDoneAt(t *testing.T) {
	lineRefused(t, `
  station {
    key       = "backlog"
    name      = "Backlog"
    kind      = "backlog"
    passes_to = ["implementation"]
  }

  station {
    key       = "implementation"
    name      = "Implementation"
    passes_to = ["completed"]
  }

  station {
    key  = "completed"
    name = "Completed"
    kind = "completed"
  }
`, regexp.MustCompile(`(?s)station\[1\]\.passes_to: must hand on to a station work is done at`))
}

func TestLineReviewsOnlyInTheReviewRole(t *testing.T) {
	lineRefused(t, `
  role = "delivery"

  station {
    key  = "backlog"
    name = "Backlog"
    kind = "backlog"
  }

  reviews {
    start = "opened"
  }
`, regexp.MustCompile(`(?s)reviews: The delivery line reviews pull requests, which only a line whose\s+role is review does`))
}

func TestLineImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            acctest.Workspace + lineExported,
				ConfigStateChecks: acctest.ExpectIdentity(lineAddress),
			},
			acctest.ImportedByIdentity(lineAddress),
		},
	})
}
