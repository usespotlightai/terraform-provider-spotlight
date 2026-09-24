package harnessprofile_test

import (
	"regexp"
	"strings"
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

const address = "spotlight_harness_profile.core_careful"

const exported = `
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

const minimal = `
resource "spotlight_harness_profile" "core_careful" {
  team_id = spotlight_team.core.id
  key     = "careful"
}
`

func importID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/core/careful", nil
}

func TestHarnessProfileExportedShapeRoundTrips(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		CheckDestroy:             providertest.NoneLeft(fake, "harness-profile", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: providertest.Workspace + exported,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("address"), knownvalue.StringExact("core/careful")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("permissions").AtMapKey("deny"), knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("read(.env)")})),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("mcp_servers").AtSliceIndex(0).AtMapKey("env").AtMapKey("LINEAR_WORKSPACE"), knownvalue.StringExact("acme")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("native").AtMapKey("claude-code").AtMapKey("includeCoAuthoredBy"), knownvalue.Bool(false)),
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
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("harness-profile", "core/careful"), func(fields map[string]any) {
						fields["native"] = map[string]any{"claude-code": map[string]any{"includeCoAuthoredBy": true}}
					})
				},
				Config: providertest.Workspace + exported,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("native").AtMapKey("codex"), knownvalue.StringExact("model_verbosity = \"low\"\n")),
				},
			},
			{
				Config: providertest.Workspace + strings.Replace(exported, `"read", "execute(npm test *)"`, `"read"`, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
			},
			{
				Config: providertest.Workspace + strings.Replace(exported, `"careful"`, `"cautious"`, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroyBeforeCreate)},
				},
			},
		},
	})
}

func TestHarnessProfileFillsWhatItLeavesOut(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config: providertest.Workspace + minimal,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("permissions").AtMapKey("allow"), knownvalue.ListSizeExact(0)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("mcp_servers"), knownvalue.ListSizeExact(0)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("native"), knownvalue.MapSizeExact(0)),
				},
			},
			{
				Config:   providertest.Workspace + minimal,
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

func TestHarnessProfileRefusalNamesTheAttribute(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config:      providertest.Workspace + strings.Replace(exported, `"read(.env)"`, `"read"`, 1),
				ExpectError: regexp.MustCompile(`(?s)permissions\.deny\[0\]: names read under both allow and deny`),
			},
		},
	})
}

func TestHarnessProfileWritesNeedTheConfigureScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config: providertest.Workspace,
			},
			{
				PreConfig:   func() { fake.Grant("secrets", "runners") },
				Config:      providertest.Workspace + minimal,
				ExpectError: regexp.MustCompile(`(?s)Not allowed to change the harness-profile.*lacks the configure scope`),
			},
			{
				PreConfig: func() { fake.Grant("configure", "secrets", "runners") },
				Config:    providertest.Workspace,
			},
		},
	})
}

func TestHarnessProfileRefusalsInsideMapsAndNativeNameTheirKeys(t *testing.T) {
	fake := fakeapi.New(t)

	for written, says := range map[string]string{
		strings.Replace(exported, "LINEAR_WORKSPACE", `"LINEAR WORKSPACE"`, 1):                                         `mcp_servers\[0\]\.env\["LINEAR WORKSPACE"\]: names an environment variable`,
		strings.Replace(exported, "codex =", "gemini =", 1):                                                            `native\.gemini: names no harness Spotlight runs`,
		strings.Replace(exported, `codex = "model_verbosity = \"low\"\n"`, "codex = { model_verbosity = \"low\" }", 1): `native\.codex: is the harness's own config\.toml, as text`,
	} {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: providertest.Factories(fake),
			Steps: []resource.TestStep{
				{
					Config:      providertest.Workspace + written,
					ExpectError: regexp.MustCompile(says),
				},
			},
		})
	}
}
