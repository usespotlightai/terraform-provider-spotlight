package provider_test

import (
	"fmt"
	"regexp"
	"strings"
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
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"
  role    = "review"
  runners = ["vpc-internal", "linux"]

  repository {
    name  = "acme/api"
    image = "ghcr.io/acme/api-runner:main"
    setup = "npm ci"
  }

  station {
    key       = "backlog"
    name      = "Backlog"
    kind      = "backlog"
    color     = "#a3a8b0"
    diffs     = false
    decisions = false
    memories  = false
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
    profile   = "careful"
    prompt    = "Implement the task.\n"
    check     = "npm test"
    diffs     = true
    decisions = true
    memories  = false
    tools     = ["linear.comment"]
    skills    = ["grafana"]
    runners   = ["gpu"]
    passes_to = ["code-review"]
    ends      = false

    judge {
      rubric        = "A good run changes only what the task requires.\n"
      sample        = 0.2
      min_agreement = 0.6
    }

    alert {
      measure  = "sent_back_rate"
      rises_by = 0.2
      over     = 20
    }

    alert {
      measure = "cost_per_run"
      above   = 5
      over    = 10
    }

    alert {
      measure  = "accepted_rate"
      falls_by = 0.1
      over     = 20
    }

    alert {
      measure = "merged_rate"
      below   = 0.4
      over    = 20
    }
  }

  station {
    key       = "code-review"
    name      = "Code review"
    kind      = "work"
    color     = "#56b98d"
    diffs     = false
    decisions = false
    memories  = false
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
    memories  = false
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
    label   = "spotlight"

    skip {
      spotlight = true
      drafts    = true
      authors   = ["dependabot[bot]", "renovate[bot]"]
    }
  }

  replay_check {
    max_regressions = 1
    ceiling         = 12.5
    currency        = "USD"
    min_gain        = 0.05
  }

  source {
    repository = "acme/platform"
    path       = "teams/core/delivery.tf"
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
    key       = "implementation"
    name      = "Implementation"
    harness   = "claude-code"
    passes_to = ["code-review"]
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

func lineRepository(name string, image, setup knownvalue.Check) knownvalue.Check {
	return knownvalue.ObjectExact(map[string]knownvalue.Check{
		"name":  knownvalue.StringExact(name),
		"image": image,
		"setup": setup,
	})
}

func lineAlert(measure, threshold string, value float64, over int64) knownvalue.Check {
	checks := map[string]knownvalue.Check{
		"measure":  knownvalue.StringExact(measure),
		"rises_by": knownvalue.Null(),
		"above":    knownvalue.Null(),
		"falls_by": knownvalue.Null(),
		"below":    knownvalue.Null(),
		"over":     knownvalue.Int64Exact(over),
	}
	checks[threshold] = knownvalue.Float64Exact(value)

	return knownvalue.ObjectExact(checks)
}

func lineReplayCheck(maxRegressions int64, ceiling float64, currency string, minGain float64) knownvalue.Check {
	return knownvalue.ObjectExact(map[string]knownvalue.Check{
		"max_regressions": knownvalue.Int64Exact(maxRegressions),
		"ceiling":         knownvalue.Float64Exact(ceiling),
		"currency":        knownvalue.StringExact(currency),
		"min_gain":        knownvalue.Float64Exact(minGain),
	})
}

func lineSource(repository, path string) knownvalue.Check {
	return knownvalue.ObjectExact(map[string]knownvalue.Check{
		"repository": knownvalue.StringExact(repository),
		"path":       knownvalue.StringExact(path),
	})
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
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("repository"), knownvalue.SetExact([]knownvalue.Check{
						lineRepository("acme/api", knownvalue.StringExact("ghcr.io/acme/api-runner:main"), knownvalue.StringExact("npm ci")),
					})),
					statecheck.ExpectKnownValue(lineAddress, lineStation(1, "runners"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("gpu")})),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("runners"), knownvalue.SetSizeExact(2)),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("reaction"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue(lineAddress, lineStation(1, "judge").AtMapKey("sample"), knownvalue.Float64Exact(0.2)),
					statecheck.ExpectKnownValue(lineAddress, lineStation(1, "alert"), knownvalue.ListExact([]knownvalue.Check{
						lineAlert("sent_back_rate", "rises_by", 0.2, 20),
						lineAlert("cost_per_run", "above", 5, 10),
						lineAlert("accepted_rate", "falls_by", 0.1, 20),
						lineAlert("merged_rate", "below", 0.4, 20),
					})),
					statecheck.ExpectKnownValue(lineAddress, lineStation(2, "judge"), knownvalue.Null()),
					statecheck.ExpectKnownValue(lineAddress, lineStation(2, "alert"), knownvalue.ListSizeExact(0)),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("replay_check"), lineReplayCheck(1, 12.5, "USD", 0.05)),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("source"), lineSource("acme/platform", "teams/core/delivery.tf")),
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
					statecheck.ExpectKnownValue(lineAddress, lineStation(1, "memories"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(lineAddress, lineStation(2, "tools"), knownvalue.ListSizeExact(0)),
					statecheck.ExpectKnownValue(lineAddress, lineStation(2, "runners"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("runners"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("reviews").AtMapKey("skip").AtMapKey("authors"), knownvalue.SetSizeExact(2)),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("reviews").AtMapKey("label"), knownvalue.StringExact("spotlight")),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("replay_check"), knownvalue.Null()),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("source"), knownvalue.Null()),
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

func TestLineStationDraftsMemories(t *testing.T) {
	fake := fakeapi.New(t)

	drafting := acctest.Workspace + `
resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"

  station {
    key      = "implementation"
    name     = "Implementation"
    memories = true
  }

  station {
    key  = "code-review"
    name = "Code review"
  }
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: drafting,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "memories"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(lineAddress, lineStation(1, "memories"), knownvalue.Bool(false)),
				},
			},
			{
				Config:   drafting,
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
							t.Fatalf("the fake stores no definition: %v", fields)
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
				PreConfig: func() { fake.Delete(fake.IDOf("line", "core/delivery")) },
				Config:    acctest.Workspace + lineMinimal,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(lineAddress, plancheck.ResourceActionCreate)},
				},
			},
		},
	})
}

func TestLineProblemNamesTheStationAttribute(t *testing.T) {
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

func lineRejected(t *testing.T, stations string, want *regexp.Regexp) {
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

func TestLineRejectsAStationKeyRunsSayToHandOver(t *testing.T) {
	lineRejected(t, `
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

func TestLinePassesOnlyToStationsWorkIsDoneAt(t *testing.T) {
	lineRejected(t, `
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
`, regexp.MustCompile(`(?s)station\[1\]\.passes_to: must pass to work stations`))
}

func TestLineReviewsOnlyInTheReviewRole(t *testing.T) {
	lineRejected(t, `
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

func lineWorking(api string) string {
	return acctest.Workspace + `
resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"

  repository {
    name  = "acme/web"
    setup = "npm ci"
  }
` + api + `
  station {
    key   = "implementation"
    name  = "Implementation"
    check = "npm test"
    diffs = true
  }
}
`
}

func TestLineRepositoriesCarryTheImageAndSetup(t *testing.T) {
	fake := fakeapi.New(t)

	pinned := lineWorking(`
  repository {
    name  = "acme/api"
    image = "ghcr.io/acme/api-runner:main"
    setup = "go mod download"
  }
`)
	moved := lineWorking(`
  repository {
    name  = "acme/api"
    image = "ghcr.io/acme/api-runner:next"
    setup = "go mod download"
  }
`)
	plain := lineWorking(`
  repository {
    name  = "acme/api"
    setup = "go mod download"
  }
`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "line", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: pinned,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("repository"), knownvalue.SetExact([]knownvalue.Check{
						lineRepository("acme/api", knownvalue.StringExact("ghcr.io/acme/api-runner:main"), knownvalue.StringExact("go mod download")),
						lineRepository("acme/web", knownvalue.Null(), knownvalue.StringExact("npm ci")),
					})),
				},
			},
			{
				Config:   pinned,
				PlanOnly: true,
			},
			{
				ResourceName:      lineAddress,
				ImportState:       true,
				ImportStateIdFunc: lineImportID,
				ImportStateVerify: true,
			},
			{
				Config: moved,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(lineAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("repository"), knownvalue.SetPartial([]knownvalue.Check{
						lineRepository("acme/api", knownvalue.StringExact("ghcr.io/acme/api-runner:next"), knownvalue.StringExact("go mod download")),
					})),
				},
			},
			{
				Config: plain,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(lineAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("repository"), knownvalue.SetPartial([]knownvalue.Check{
						lineRepository("acme/api", knownvalue.Null(), knownvalue.StringExact("go mod download")),
					})),
				},
			},
			{
				Config:   plain,
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

func TestLineKeepsAScriptOfManyLines(t *testing.T) {
	fake := fakeapi.New(t)

	scripted := acctest.Workspace + `
resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"

  repository {
    name  = "acme/web"
    setup = <<-EOT
      set -e
      npm ci
      npm run build
    EOT
  }

  station {
    key   = "implementation"
    name  = "Implementation"
    check = <<-EOT
      set -e
      if [ -f GNUmakefile ]; then
        make check
      else
        npm test
      fi
    EOT
    diffs = true
  }
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "line", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: scripted,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("repository"), knownvalue.SetExact([]knownvalue.Check{
						lineRepository("acme/web", knownvalue.Null(), knownvalue.StringExact("set -e\nnpm ci\nnpm run build\n")),
					})),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("station").AtSliceIndex(0).AtMapKey("check"),
						knownvalue.StringExact("set -e\nif [ -f GNUmakefile ]; then\n  make check\nelse\n  npm test\nfi\n")),
				},
			},
			{
				Config:   scripted,
				PlanOnly: true,
			},
		},
	})
}

func TestLineRejectsACheckLongerThanTheLimitAtPlan(t *testing.T) {
	lineRejected(t, `
  station {
    key   = "implementation"
    name  = "Implementation"
    check = "`+strings.Repeat("x", 65537)+`"
  }
`, regexp.MustCompile(`(?s)station\[0\]\.check.*string length must be between 1 and 65536`))
}

func TestLineRunsInOneImage(t *testing.T) {
	lineRejected(t, `
  repository {
    name  = "acme/api"
    image = "ghcr.io/acme/api-runner:main"
  }

  repository {
    name  = "acme/web"
    image = "ghcr.io/acme/web-runner:main"
  }

  station {
    key  = "implementation"
    name = "Implementation"
  }
`, regexp.MustCompile(`(?s)repository: The delivery line works in repositories that A line runs every\s+station in one image`))
}

func TestLineRejectsAnImageThatIsNoReference(t *testing.T) {
	lineRejected(t, `
  repository {
    name  = "acme/api"
    image = "GHCR.io/Acme/API"
  }

  station {
    key  = "implementation"
    name = "Implementation"
  }
`, regexp.MustCompile(`(?s)repository: A runner image is named by a reference`))
}

func TestLineThatChecksLayersSetsUpEveryRepository(t *testing.T) {
	lineRejected(t, `
  repository {
    name = "acme/api"
  }

  station {
    key   = "implementation"
    name  = "Implementation"
    check = "npm test"
    diffs = true
  }
`, regexp.MustCompile(`(?s)repository: The delivery line has implementation, which checks its layers,\s+so\s+acme/api has to say how its checkout is made ready`))
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

func lineJudged(station string) string {
	return acctest.Workspace + `
resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"

  station {
    key  = "implementation"
    name = "Implementation"
` + station + `
  }
}
`
}

func TestLineStationsJudgeAndAlert(t *testing.T) {
	fake := fakeapi.New(t)

	judged := lineJudged(`
    judge {
      rubric        = "A good run changes only what the task requires."
      sample        = 0.2
      min_agreement = 0.6
    }

    alert {
      measure  = "sent_back_rate"
      rises_by = 0.2
      over     = 20
    }
`)
	retuned := lineJudged(`
    judge {
      rubric        = "A good run changes only what the task requires."
      sample        = 1
      min_agreement = 0.6
    }

    alert {
      measure = "failed_rate"
      above   = 0.1
      over    = 5
    }

    alert {
      measure  = "sent_back_rate"
      rises_by = 0.2
      over     = 20
    }

    alert {
      measure  = "accepted_rate"
      falls_by = 0.05
      over     = 20
    }

    alert {
      measure = "merged_rate"
      below   = 0
      over    = 20
    }
`)
	unjudged := lineJudged("")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "line", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: judged,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "judge"), knownvalue.ObjectExact(map[string]knownvalue.Check{
						"rubric":         knownvalue.StringExact("A good run changes only what the task requires."),
						"claims":         knownvalue.Null(),
						"pass_threshold": knownvalue.Null(),
						"sample":         knownvalue.Float64Exact(0.2),
						"min_agreement":  knownvalue.Float64Exact(0.6),
					})),
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "alert"), knownvalue.ListExact([]knownvalue.Check{
						lineAlert("sent_back_rate", "rises_by", 0.2, 20),
					})),
				},
			},
			{
				Config:   judged,
				PlanOnly: true,
			},
			{
				ResourceName:      lineAddress,
				ImportState:       true,
				ImportStateIdFunc: lineImportID,
				ImportStateVerify: true,
			},
			{
				Config: retuned,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(lineAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "judge").AtMapKey("sample"), knownvalue.Float64Exact(1)),
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "alert"), knownvalue.ListExact([]knownvalue.Check{
						lineAlert("failed_rate", "above", 0.1, 5),
						lineAlert("sent_back_rate", "rises_by", 0.2, 20),
						lineAlert("accepted_rate", "falls_by", 0.05, 20),
						lineAlert("merged_rate", "below", 0, 20),
					})),
				},
			},
			{
				Config:   retuned,
				PlanOnly: true,
			},
			{
				Config: unjudged,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(lineAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "judge"), knownvalue.Null()),
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "alert"), knownvalue.ListSizeExact(0)),
				},
			},
			{
				Config:   unjudged,
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

func TestLineJudgeChecksClaims(t *testing.T) {
	fake := fakeapi.New(t)

	claimed := lineJudged(`
    judge {
      claims = [
        "The change touches only files the task names.",
        "Every new function has a test.",
      ]
      sample        = 0.2
      min_agreement = 0.6
    }
`)
	shared := lineJudged(`
    judge {
      rubric         = "A good run changes only what the task requires."
      claims         = ["The change touches only files the task names."]
      pass_threshold = 0.75
      sample         = 0.2
      min_agreement  = 0.6
    }
`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "line", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: claimed,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "judge"), knownvalue.ObjectExact(map[string]knownvalue.Check{
						"rubric": knownvalue.Null(),
						"claims": knownvalue.ListExact([]knownvalue.Check{
							knownvalue.StringExact("The change touches only files the task names."),
							knownvalue.StringExact("Every new function has a test."),
						}),
						"pass_threshold": knownvalue.Float64Exact(1),
						"sample":         knownvalue.Float64Exact(0.2),
						"min_agreement":  knownvalue.Float64Exact(0.6),
					})),
				},
			},
			{
				Config:   claimed,
				PlanOnly: true,
			},
			{
				ResourceName:      lineAddress,
				ImportState:       true,
				ImportStateIdFunc: lineImportID,
				ImportStateVerify: true,
			},
			{
				Config: shared,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(lineAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "judge").AtMapKey("rubric"), knownvalue.StringExact("A good run changes only what the task requires.")),
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "judge").AtMapKey("pass_threshold"), knownvalue.Float64Exact(0.75)),
				},
			},
			{
				Config:   shared,
				PlanOnly: true,
			},
			{
				Config: claimed,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "judge").AtMapKey("rubric"), knownvalue.Null()),
					statecheck.ExpectKnownValue(lineAddress, lineStation(0, "judge").AtMapKey("pass_threshold"), knownvalue.Float64Exact(1)),
				},
			},
			{
				Config:   claimed,
				PlanOnly: true,
			},
		},
	})
}

func TestLineJudgeSetsARubricOrClaims(t *testing.T) {
	lineInvalid(t, `
    judge {
      sample        = 0.2
      min_agreement = 0.6
    }
`, regexp.MustCompile(`(?s)Missing Judge Criteria.*rubric.*claims`))
}

func TestLineJudgePassThresholdNeedsClaims(t *testing.T) {
	lineInvalid(t, `
    judge {
      rubric         = "A good run."
      pass_threshold = 0.5
      sample         = 0.2
      min_agreement  = 0.6
    }
`, regexp.MustCompile(`(?s)station\[0\]\.judge\.claims"?\s+must be specified when\s+"?station\[0\]\.judge\.pass_threshold`))
}

func TestLineJudgePassThresholdIsAShare(t *testing.T) {
	lineInvalid(t, `
    judge {
      claims         = ["Every new function has a test."]
      pass_threshold = 1.5
      sample         = 0.2
      min_agreement  = 0.6
    }
`, regexp.MustCompile(`(?s)station\[0\]\.judge\.pass_threshold value must be between 0\.0+ and\s+1\.0+`))
}

func TestLineJudgeClaimsAreFew(t *testing.T) {
	claims := make([]string, 21)
	for at := range claims {
		claims[at] = fmt.Sprintf("%q", fmt.Sprintf("Claim %d is true.", at))
	}

	lineInvalid(t, `
    judge {
      claims        = [`+strings.Join(claims, ", ")+`]
      sample        = 0.2
      min_agreement = 0.6
    }
`, regexp.MustCompile(`(?s)station\[0\]\.judge\.claims list must contain at least 1 elements and\s+at most 20`))
}

func TestLineJudgeClaimsAreShortAndNotEmpty(t *testing.T) {
	for _, claim := range []string{`""`, fmt.Sprintf("%q", strings.Repeat("a", 501))} {
		lineInvalid(t, `
    judge {
      claims        = [`+claim+`]
      sample        = 0.2
      min_agreement = 0.6
    }
`, regexp.MustCompile(`(?s)station\[0\]\.judge\.claims\[0\].*between 1 and 500`))
	}
}

func lineInvalid(t *testing.T, station string, want *regexp.Regexp) {
	t.Helper()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fakeapi.New(t)),
		Steps: []resource.TestStep{
			{
				Config:      lineJudged(station),
				PlanOnly:    true,
				ExpectError: want,
			},
		},
	})
}

func TestLineJudgeSamplesAShare(t *testing.T) {
	lineInvalid(t, `
    judge {
      rubric        = "A good run."
      sample        = 1.5
      min_agreement = 0.6
    }
`, regexp.MustCompile(`(?s)station\[0\]\.judge\.sample value must be between 0\.0+ and\s+1\.0+`))
}

func TestLineJudgeSetsItsRubricSampleAndMinAgreement(t *testing.T) {
	lineInvalid(t, `
    judge {
      rubric = "A good run."
      sample = 0.2
    }
`, regexp.MustCompile(`(?s)station\[0\]\.judge\.min_agreement.*must be specified`))
}

func TestLineAlertWatchesAKnownMeasure(t *testing.T) {
	lineInvalid(t, `
    alert {
      measure  = "happiness"
      rises_by = 0.2
      over     = 20
    }
`, regexp.MustCompile(`(?s)station\[0\]\.alert\[0\]\.measure.*sent_back_rate`))
}

func TestLineAlertSetsExactlyOneThreshold(t *testing.T) {
	for _, alert := range []string{`
    alert {
      measure  = "merged_rate"
      rises_by = 0.2
      above    = 0.5
      over     = 20
    }
`, `
    alert {
      measure  = "accepted_rate"
      falls_by = 0.1
      below    = 0.4
      over     = 20
    }
`, `
    alert {
      measure = "merged_rate"
      over    = 20
    }
`} {
		lineInvalid(t, alert, regexp.MustCompile(`(?s)Invalid Attribute Combination.*falls_by`))
	}
}

func TestLineAlertThresholdsAreNotNegative(t *testing.T) {
	for _, threshold := range []string{"rises_by", "above", "falls_by", "below"} {
		lineInvalid(t, fmt.Sprintf(`
    alert {
      measure = "cost_per_run"
      %s = -1
      over    = 10
    }
`, threshold), regexp.MustCompile(`(?s)station\[0\]\.alert\[0\]\.`+threshold+`.*at least 0`))
	}
}

func TestLineJudgeRubricIsNotEmpty(t *testing.T) {
	lineInvalid(t, `
    judge {
      rubric        = ""
      sample        = 0.2
      min_agreement = 0.6
    }
`, regexp.MustCompile(`(?s)station\[0\]\.judge\.rubric.*at least 1`))
}

func TestLineAlertComparesAtLeastOneRun(t *testing.T) {
	lineInvalid(t, `
    alert {
      measure = "checks_failed_rate"
      above   = 0.1
      over    = 0
    }
`, regexp.MustCompile(`(?s)station\[0\]\.alert\[0\]\.over.*at least 1`))
}

func lineJudgedAccConfig(k acctest.Keys, sample string, alert string) string {
	return fmt.Sprintf(`
resource "spotlight_team" "acc" {
  workspace_id = local.workspace_id
  key          = %[1]q
  name         = "Terraform acceptance %[1]s"
}

resource "spotlight_line" "delivery" {
  team_id = spotlight_team.acc.id
  key     = %[2]q
  name    = "Delivery"

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
    prompt  = "Implement the task."

    judge {
      rubric        = "A good run changes only what the task requires."
      sample        = %[3]s
      min_agreement = 0.6
    }
%[4]s
  }

  station {
    key  = "completed"
    name = "Completed"
    kind = "completed"
  }
}
`, k.Team, k.Line, sample, alert)
}

func TestAccLineStationJudgeAndAlerts(t *testing.T) {
	cp := acctest.PreCheck(t)
	k := acctest.RandomKeys()
	cp.Sweep(t, k)

	id := cp.Workspace.ID
	judged := acctest.AdoptedWorkspace(cp.Workspace) + lineJudgedAccConfig(k, "0.2", `
    alert {
      measure  = "sent_back_rate"
      rises_by = 0.2
      over     = 20
    }
`)
	retunedLine := lineJudgedAccConfig(k, "0.35", `
    alert {
      measure = "cost_per_run"
      above   = 2.5
      over    = 10
    }

    alert {
      measure  = "accepted_rate"
      falls_by = 0.1
      over     = 20
    }

    alert {
      measure = "merged_rate"
      below   = 0.4
      over    = 20
    }

    alert {
      measure  = "sent_back_rate"
      rises_by = 0.2
      over     = 20
    }
`)
	retuned := acctest.AdoptedWorkspace(cp.Workspace) + retunedLine

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config: judged,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("spotlight_line.delivery", lineStation(1, "judge").AtMapKey("min_agreement"), knownvalue.Float64Exact(0.6)),
					statecheck.ExpectKnownValue("spotlight_line.delivery", lineStation(1, "alert"), knownvalue.ListExact([]knownvalue.Check{
						lineAlert("sent_back_rate", "rises_by", 0.2, 20),
					})),
				},
			},
			{
				Config:           judged,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config: retuned,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("spotlight_line.delivery", plancheck.ResourceActionUpdate),
				}},
			},
			{
				Config:           retuned,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      "spotlight_line.delivery",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Team+"/"+k.Line),
				ImportStateVerify: true,
			},
			{
				Config:           acctest.ReleasedWorkspace(cp.Workspace) + retunedLine,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{acctest.ExpectForgotten("spotlight_workspace.acc")}},
			},
		},
	})
}

func lineChecked(blocks string) string {
	return acctest.Workspace + `
resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"

  station {
    key  = "implementation"
    name = "Implementation"
  }
` + blocks + `
}
`
}

func TestLineReplayCheckAndSource(t *testing.T) {
	fake := fakeapi.New(t)

	defaulted := lineChecked(`
  replay_check {}

  source {
    repository = "acme/infra"
    path       = "spotlight/web.tf"
  }
`)
	tightened := lineChecked(`
  replay_check {
    max_regressions = 2
    ceiling         = 0.75
    currency        = "EUR"
    min_gain        = 0.1
  }

  source {
    repository = "acme/platform"
    path       = "teams/core/delivery.tf"
  }
`)
	unchecked := lineChecked("")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "line", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: defaulted,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("replay_check"), lineReplayCheck(0, 10, "USD", 0.05)),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("source"), lineSource("acme/infra", "spotlight/web.tf")),
				},
			},
			{
				Config:   defaulted,
				PlanOnly: true,
			},
			{
				ResourceName:      lineAddress,
				ImportState:       true,
				ImportStateIdFunc: lineImportID,
				ImportStateVerify: true,
			},
			{
				Config: tightened,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(lineAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("replay_check"), lineReplayCheck(2, 0.75, "EUR", 0.1)),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("source"), lineSource("acme/platform", "teams/core/delivery.tf")),
				},
			},
			{
				Config:   tightened,
				PlanOnly: true,
			},
			{
				Config: unchecked,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(lineAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("replay_check"), knownvalue.Null()),
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("source"), knownvalue.Null()),
				},
			},
			{
				Config:   unchecked,
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

func lineCheckInvalid(t *testing.T, blocks string, want *regexp.Regexp) {
	t.Helper()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fakeapi.New(t)),
		Steps: []resource.TestStep{
			{
				Config:      lineChecked(blocks),
				PlanOnly:    true,
				ExpectError: want,
			},
		},
	})
}

func TestLineReplayCheckAllowsNoNegativeRegressions(t *testing.T) {
	lineCheckInvalid(t, `
  replay_check {
    max_regressions = -1
  }
`, regexp.MustCompile(`(?s)replay_check\.max_regressions.*at least 0`))
}

func TestLineReplayCheckCeilingIsAboveZeroAndAtMostAThousand(t *testing.T) {
	for _, ceiling := range []string{"0", "-5", "1000.5"} {
		lineCheckInvalid(t, `
  replay_check {
    ceiling = `+ceiling+`
  }
`, regexp.MustCompile(`(?s)replay_check\.ceiling`))
	}
}

func TestLineReplayCheckMinGainIsAboveZeroAndAtMostOne(t *testing.T) {
	for _, gain := range []string{"0", "-0.1", "1.5"} {
		lineCheckInvalid(t, `
  replay_check {
    min_gain = `+gain+`
  }
`, regexp.MustCompile(`(?s)replay_check\.min_gain`))
	}
}

func TestLineReplayCheckCurrencyIsThreeCapitalLetters(t *testing.T) {
	for _, currency := range []string{"usd", "US", "EURO"} {
		lineCheckInvalid(t, fmt.Sprintf(`
  replay_check {
    currency = %q
  }
`, currency), regexp.MustCompile(`(?s)replay_check\.currency.*three-letter currency`))
	}
}

func TestLineSourceSetsItsRepositoryAndPath(t *testing.T) {
	lineCheckInvalid(t, `
  source {
    repository = "acme/infra"
  }
`, regexp.MustCompile(`(?s)source\.path.*must be specified`))
}

func TestLineSourceRepositoryIsOwnerAndName(t *testing.T) {
	for _, repository := range []string{"infra", "acme/infra/web", "acme/in fra"} {
		lineCheckInvalid(t, fmt.Sprintf(`
  source {
    repository = %q
    path       = "spotlight/web.tf"
  }
`, repository), regexp.MustCompile(`(?s)source\.repository.*owner/name`))
	}
}

func TestLineSourcePathIsATerraformFileInTheRepository(t *testing.T) {
	for _, path := range []string{"spotlight/web.yml", "/spotlight/web.tf", "spotlight/../web.tf", "./web.tf", "spotlight//web.tf", `spotlight\\web.tf`} {
		lineCheckInvalid(t, fmt.Sprintf(`
  source {
    repository = "acme/infra"
    path       = %q
  }
`, path), regexp.MustCompile(`(?s)source\.path.*Terraform file ending in \.tf`))
	}
}

func TestLineSourcePathMayContainDots(t *testing.T) {
	fake := fakeapi.New(t)

	dotted := lineChecked(`
  source {
    repository = "acme/infra.config"
    path       = "...shared/.spotlight/web.tf"
  }
`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: dotted,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(lineAddress, tfjsonpath.New("source"), lineSource("acme/infra.config", "...shared/.spotlight/web.tf")),
				},
			},
			{
				Config:   dotted,
				PlanOnly: true,
			},
		},
	})
}

func lineCheckedAccConfig(k acctest.Keys, blocks string) string {
	return fmt.Sprintf(`
resource "spotlight_team" "acc" {
  workspace_id = local.workspace_id
  key          = %[1]q
  name         = "Terraform acceptance %[1]s"
}

resource "spotlight_line" "delivery" {
  team_id = spotlight_team.acc.id
  key     = %[2]q
  name    = "Delivery"

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
    prompt  = "Implement the task."
  }

  station {
    key  = "completed"
    name = "Completed"
    kind = "completed"
  }
%[3]s
}
`, k.Team, k.Line, blocks)
}

func TestAccLineReplayCheckAndSource(t *testing.T) {
	cp := acctest.PreCheck(t)
	k := acctest.RandomKeys()
	cp.Sweep(t, k)

	id := cp.Workspace.ID
	defaulted := acctest.AdoptedWorkspace(cp.Workspace) + lineCheckedAccConfig(k, `
  replay_check {}

  source {
    repository = "acme/infra"
    path       = "spotlight/web.tf"
  }
`)
	tightenedLine := lineCheckedAccConfig(k, `
  replay_check {
    max_regressions = 2
    ceiling         = 12.5
    currency        = "EUR"
    min_gain        = 0.1
  }
`)
	tightened := acctest.AdoptedWorkspace(cp.Workspace) + tightenedLine

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config: defaulted,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("spotlight_line.delivery", tfjsonpath.New("replay_check"), lineReplayCheck(0, 10, "USD", 0.05)),
					statecheck.ExpectKnownValue("spotlight_line.delivery", tfjsonpath.New("source"), lineSource("acme/infra", "spotlight/web.tf")),
				},
			},
			{
				Config:           defaulted,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      "spotlight_line.delivery",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Team+"/"+k.Line),
				ImportStateVerify: true,
			},
			{
				Config: tightened,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("spotlight_line.delivery", plancheck.ResourceActionUpdate),
				}},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("spotlight_line.delivery", tfjsonpath.New("replay_check"), lineReplayCheck(2, 12.5, "EUR", 0.1)),
					statecheck.ExpectKnownValue("spotlight_line.delivery", tfjsonpath.New("source"), knownvalue.Null()),
				},
			},
			{
				Config:           tightened,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      "spotlight_line.delivery",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Team+"/"+k.Line),
				ImportStateVerify: true,
			},
			{
				Config:           acctest.ReleasedWorkspace(cp.Workspace) + tightenedLine,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{acctest.ExpectForgotten("spotlight_workspace.acc")}},
			},
		},
	})
}
