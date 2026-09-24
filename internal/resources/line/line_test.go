package line_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/providertest"
)

const address = "spotlight_line.core_delivery"

const exported = `
resource "spotlight_line" "core_delivery" {
  team_id      = spotlight_team.core.id
  key          = "delivery"
  name         = "Delivery"
  role         = "delivery"
  repositories = ["acme/api"]

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
    key       = "implementation"
    name      = "Implementation"
    kind      = "work"
    color     = "#6d78d5"
    harness   = "claude-code"
    prompt    = "Implement the task.\n"
    check     = "npm test"
    diffs     = true
    decisions = true
    tools     = ["linear.comment"]
    skills    = ["grafana"]
    passes_to = ["completed"]
    ends      = false
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
    ends      = true
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
      authors   = ["dependabot[bot]"]
    }
  }
}
`

const minimal = `
resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"

  station {
    key       = "backlog"
    name      = "Backlog"
    kind      = "backlog"
    passes_to = ["implementation"]
  }

  station {
    key       = "implementation"
    name      = "Implementation"
    harness   = "claude-code"
    passes_to = ["completed"]
  }

  station {
    key  = "completed"
    name = "Completed"
    kind = "completed"
    ends = true
  }

  reviews {
    start = "opened"
  }
}
`

const grown = `
resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"

  station {
    key       = "backlog"
    name      = "Backlog"
    kind      = "backlog"
    passes_to = ["implementation"]
  }

  station {
    key       = "implementation"
    name      = "Implementation"
    harness   = "claude-code"
    passes_to = ["review"]
  }

  station {
    key       = "review"
    name      = "Review"
    passes_to = ["completed", "implementation"]
  }

  station {
    key  = "completed"
    name = "Completed"
    kind = "completed"
    ends = true
  }

  reviews {
    start = "opened"
  }
}
`

func importID(state *terraform.State) (string, error) {
	workspace := state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID

	return workspace + "/core/delivery", nil
}

func station(at int, name string) tfjsonpath.Path {
	return tfjsonpath.New("station").AtSliceIndex(at).AtMapKey(name)
}

func TestLineExportedShapeRoundTrips(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		CheckDestroy:             providertest.NoneLeft(fake, "line", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: providertest.Workspace + exported,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("address"), knownvalue.StringExact("core/delivery")),
					statecheck.ExpectKnownValue(address, station(1, "passes_to"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("completed")})),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("reaction"), knownvalue.SetSizeExact(1)),
				},
			},
			{
				Config:   providertest.Workspace + exported,
				PlanOnly: true,
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateIdFunc: importID,
				ImportStateVerify: true,
			},
		},
	})
}

func TestLineFillsWhatItLeavesOutWithoutDrift(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config: providertest.Workspace + minimal,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, station(0, "color"), knownvalue.StringExact("#a3a8b0")),
					statecheck.ExpectKnownValue(address, station(1, "kind"), knownvalue.StringExact("work")),
					statecheck.ExpectKnownValue(address, station(1, "diffs"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(address, station(2, "tools"), knownvalue.ListSizeExact(0)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("reviews").AtMapKey("skip"), knownvalue.Null()),
				},
			},
			{
				Config:   providertest.Workspace + minimal,
				PlanOnly: true,
			},
			{
				Config: providertest.Workspace + grown,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, station(2, "color"), knownvalue.StringExact("#6d78d5")),
					statecheck.ExpectKnownValue(address, station(3, "color"), knownvalue.StringExact("#3fa46a")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("version"), knownvalue.Int64Exact(2)),
				},
			},
		},
	})
}

func TestLineDriftIsPlannedBack(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config: providertest.Workspace + minimal,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("line", "core/delivery"), func(fields map[string]any) {
						definition := fields["definition"].(map[string]any)
						definition["name"] = "Renamed in the web app"
					})
				},
				Config: providertest.Workspace + minimal,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("name"), knownvalue.StringExact("Delivery")),
				},
			},
			{
				PreConfig: func() { fake.Forget(fake.IDOf("line", "core/delivery")) },
				Config:    providertest.Workspace + minimal,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionCreate)},
				},
			},
		},
	})
}

func TestLineRefusalNamesTheStationAttribute(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config: providertest.Workspace + `
resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"

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
