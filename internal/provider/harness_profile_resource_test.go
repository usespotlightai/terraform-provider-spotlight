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

const harnessProfileAddress = "spotlight_harness_profile.core_careful"

const harnessProfileExported = `
resource "spotlight_harness_profile" "core_careful" {
  team_id = spotlight_team.core.id
  key     = "careful"
  permissions = {
    allow = ["read", "execute(npm test *)"]
    ask   = []
    deny  = ["read(.env)"]
  }
  mcp_servers = [
    {
      args    = ["-y", "linear-mcp"]
      command = "npx"
      env = {
        LINEAR_WORKSPACE = "acme"
      }
      name = "linear"
    },
  ]
  native = {
    claude-code = {
      includeCoAuthoredBy = false
    }
    codex = "model_verbosity = \"low\"\n"
  }
}
`

const harnessProfileMinimal = `
resource "spotlight_harness_profile" "core_careful" {
  team_id = spotlight_team.core.id
  key     = "careful"
}
`

func harnessProfileImportID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/core/careful", nil
}

func TestHarnessProfileExportedShapeRoundTrips(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "harness-profile", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + harnessProfileExported,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(harnessProfileAddress, tfjsonpath.New("address"), knownvalue.StringExact("core/careful")),
					statecheck.ExpectKnownValue(harnessProfileAddress, tfjsonpath.New("permissions").AtMapKey("deny"), knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("read(.env)")})),
					statecheck.ExpectKnownValue(harnessProfileAddress, tfjsonpath.New("mcp_servers").AtSliceIndex(0).AtMapKey("env").AtMapKey("LINEAR_WORKSPACE"), knownvalue.StringExact("acme")),
					statecheck.ExpectKnownValue(harnessProfileAddress, tfjsonpath.New("native").AtMapKey("claude-code").AtMapKey("includeCoAuthoredBy"), knownvalue.Bool(false)),
				},
			},
			{
				Config:   acctest.Workspace + harnessProfileExported,
				PlanOnly: true,
			},
			{
				ResourceName:      harnessProfileAddress,
				ImportState:       true,
				ImportStateIdFunc: harnessProfileImportID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("harness-profile", "core/careful"), func(fields map[string]any) {
						fields["native"] = map[string]any{"claude-code": map[string]any{"includeCoAuthoredBy": true}}
					})
				},
				Config: acctest.Workspace + harnessProfileExported,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(harnessProfileAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(harnessProfileAddress, tfjsonpath.New("native").AtMapKey("codex"), knownvalue.StringExact("model_verbosity = \"low\"\n")),
				},
			},
			{
				Config: acctest.Workspace + strings.Replace(harnessProfileExported, `"read", "execute(npm test *)"`, `"read"`, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(harnessProfileAddress, plancheck.ResourceActionUpdate)},
				},
			},
			{
				Config: acctest.Workspace + strings.Replace(harnessProfileExported, `"careful"`, `"cautious"`, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(harnessProfileAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
			},
		},
	})
}

func TestHarnessProfileFillsWhatItLeavesOut(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + harnessProfileMinimal,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(harnessProfileAddress, tfjsonpath.New("permissions").AtMapKey("allow"), knownvalue.ListSizeExact(0)),
					statecheck.ExpectKnownValue(harnessProfileAddress, tfjsonpath.New("mcp_servers"), knownvalue.ListSizeExact(0)),
					statecheck.ExpectKnownValue(harnessProfileAddress, tfjsonpath.New("native"), knownvalue.MapSizeExact(0)),
				},
			},
			{
				Config:   acctest.Workspace + harnessProfileMinimal,
				PlanOnly: true,
			},
			{
				ResourceName:      harnessProfileAddress,
				ImportState:       true,
				ImportStateIdFunc: harnessProfileImportID,
				ImportStateVerify: true,
			},
		},
	})
}

func TestHarnessProfileProblemNamesTheAttribute(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      acctest.Workspace + strings.Replace(harnessProfileExported, `"read(.env)"`, `"read"`, 1),
				ExpectError: regexp.MustCompile(`(?s)permissions\.deny\[0\]: names read under both allow and deny`),
			},
		},
	})
}

func TestHarnessProfileWritesNeedTheConfigureScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace,
			},
			{
				PreConfig:   func() { fake.Grant("secrets", "runners") },
				Config:      acctest.Workspace + harnessProfileMinimal,
				ExpectError: regexp.MustCompile(`(?s)Not allowed to change the harness-profile.*lacks the configure scope`),
			},
			{
				PreConfig: func() { fake.Grant("configure", "secrets", "runners") },
				Config:    acctest.Workspace,
			},
		},
	})
}

func TestHarnessProfileProblemsInsideMapsAndNativeNameTheirKeys(t *testing.T) {
	fake := fakeapi.New(t)

	for written, says := range map[string]string{
		strings.Replace(harnessProfileExported, "LINEAR_WORKSPACE", `"LINEAR WORKSPACE"`, 1):                                         `mcp_servers\[0\]\.env\["LINEAR WORKSPACE"\]: names an environment variable`,
		strings.Replace(harnessProfileExported, "codex =", "gemini =", 1):                                                            `native\.gemini: names no harness Spotlight runs`,
		strings.Replace(harnessProfileExported, `codex = "model_verbosity = \"low\"\n"`, "codex = { model_verbosity = \"low\" }", 1): `native\.codex: is the harness's own config\.toml, as text`,
	} {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
			Steps: []resource.TestStep{
				{
					Config:      acctest.Workspace + written,
					ExpectError: regexp.MustCompile(says),
				},
			},
		})
	}
}

func harnessProfileAccConfig(k acctest.Keys, allow string, prompt string) string {
	return fmt.Sprintf(`
resource "spotlight_team" "acc" {
  workspace_id = local.workspace_id
  key          = %[1]q
  name         = "Terraform acceptance %[1]s"
}

resource "spotlight_harness_profile" "careful" {
  team_id = spotlight_team.acc.id
  key     = %[2]q
  permissions = {
    allow = [%[4]s]
    deny  = ["read(.env)"]
  }
  mcp_servers = [
    {
      name    = "linear"
      command = "npx"
      args    = ["-y", "linear-mcp"]
      env     = { LINEAR_WORKSPACE = "tf-acc" }
    },
  ]
  native = {
    claude-code = {
      includeCoAuthoredBy = false
    }
  }
}

resource "spotlight_line" "delivery" {
  team_id = spotlight_team.acc.id
  key     = %[3]q
  name    = "Review"
  role    = "review"
  runners = ["tf-acc-vpc"]

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
    profile   = spotlight_harness_profile.careful.key
    prompt    = %[5]q
    runners   = ["tf-acc-gpu"]
    passes_to = ["code-review"]
  }

  station {
    key       = "code-review"
    name      = "Code review"
    harness   = "claude-code"
    prompt    = "Review the change."
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
      authors = ["renovate[bot]", "dependabot[bot]"]
    }
  }
}
`, k.Team, k.Profile, k.Line, allow, prompt)
}

func TestAccHarnessProfileAndPlacedLine(t *testing.T) {
	cp := acctest.PreCheck(t)
	k := acctest.RandomKeys()
	cp.Sweep(t, k)

	id := cp.Workspace.ID

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config: acctest.AdoptedWorkspace(cp.Workspace) + harnessProfileAccConfig(k, `"read"`, "Implement the task."),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_harness_profile.careful", "address", k.Team+"/"+k.Profile),
					resource.TestCheckResourceAttr("spotlight_harness_profile.careful", "permissions.ask.#", "0"),
					resource.TestCheckResourceAttr("spotlight_line.delivery", "station.1.profile", k.Profile),
					resource.TestCheckResourceAttr("spotlight_line.delivery", "station.1.runners.0", "tf-acc-gpu"),
					resource.TestCheckResourceAttr("spotlight_line.delivery", "runners.0", "tf-acc-vpc"),
					resource.TestCheckResourceAttr("spotlight_line.delivery", "reviews.skip.authors.#", "2"),
				),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + harnessProfileAccConfig(k, `"read"`, "Implement the task."),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config: acctest.AdoptedWorkspace(cp.Workspace) + harnessProfileAccConfig(k, `"read", "execute(npm test *)"`, "Implement the task carefully."),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("spotlight_harness_profile.careful", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("spotlight_line.delivery", plancheck.ResourceActionUpdate),
				}},
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + harnessProfileAccConfig(k, `"read", "execute(npm test *)"`, "Implement the task carefully."),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      "spotlight_harness_profile.careful",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Team+"/"+k.Profile),
				ImportStateVerify: true,
			},
			{
				ResourceName:      "spotlight_line.delivery",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Team+"/"+k.Line),
				ImportStateVerify: true,
			},
			{
				Config: acctest.ReleasedWorkspace(cp.Workspace) + harnessProfileAccConfig(k, `"read", "execute(npm test *)"`, "Implement the task carefully."),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					acctest.ExpectForgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction("spotlight_harness_profile.careful", plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}

func TestHarnessProfileImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            acctest.Workspace + harnessProfileExported,
				ConfigStateChecks: acctest.ExpectIdentity(harnessProfileAddress),
			},
			acctest.ImportedByIdentity(harnessProfileAddress),
		},
	})
}
