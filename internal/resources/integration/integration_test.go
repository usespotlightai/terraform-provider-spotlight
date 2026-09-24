package integration_test

import (
	"fmt"
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

const address = "spotlight_integration.core_linear"

func config(host, when, thread string) string {
	return providertest.Workspace + fmt.Sprintf(`
resource "spotlight_integration" "core_linear" {
  team_id = spotlight_team.core.id
  definition = {
    integration = %q
    webhook = {
      verify = { hmac = { secret = "secret.LINEAR_SIGNING" } }
      events = [
        {
          when = %q
          raise = {
            happening = "linear.issue.completed"
            marks     = [{ kind = "linear.issue", value = "$${{ body.data.id }}" }]
          }
        }
      ]
    }
    actions = {
      comment = {
        request = {
          url  = "https://api.linear.app/graphql"
          body = { query = "$${{ inputs.body }}" }
        }
      }
    }
    on = {
      "task.opened" = {
        do = "comment"
        to = { thread = %q }
      }
    }
  }
}
`, host, when, thread)
}

const (
	soundWhen   = `$${{ body.type == "Issue" }}`
	soundThread = `$${{ ask.body }}`
)

func importID(state *terraform.State) (string, error) {
	workspace := state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID

	return workspace + "/core/linear", nil
}

func TestIntegrationLifecycle(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		CheckDestroy:             providertest.NoneLeft(fake, "integration", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: config("linear", soundWhen, soundThread),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("source"), knownvalue.StringExact("linear")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("address"), knownvalue.StringExact("core/linear")),
					statecheck.ExpectKnownValue(address,
						tfjsonpath.New("definition").AtMapKey("webhook").AtMapKey("events").AtSliceIndex(0).AtMapKey("when"),
						knownvalue.StringExact(`${{ body.type == "Issue" }}`)),
				},
			},
			{
				Config:   config("linear", soundWhen, soundThread),
				PlanOnly: true,
			},
			{
				ResourceName:            address,
				ImportState:             true,
				ImportStateIdFunc:       importID,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"definition"},
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("integration", "core/linear"), func(fields map[string]any) {
						definition := fields["definition"].(map[string]any)
						definition["on"] = map[string]any{}
					})
				},
				Config: config("linear", soundWhen, soundThread),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
			},
			{
				Config: config("jira", soundWhen, soundThread),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroyBeforeCreate),
						plancheck.ExpectKnownValue(address, tfjsonpath.New("source"), knownvalue.StringExact("jira")),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("address"), knownvalue.StringExact("core/jira")),
				},
			},
		},
	})
}

func TestIntegrationImportedAsExportedHasNoDiff(t *testing.T) {
	fake := fakeapi.New(t)

	exported := providertest.Workspace + `
resource "spotlight_integration" "core_linear" {
  team_id = spotlight_team.core.id
  definition = {
    actions = {
      comment = {
        request = {
          headers = {
            "Content-Type" = "application/json"
          }
          method = "POST"
          url    = "https://api.linear.app/graphql"
        }
      }
    }
    integration = "linear"
    poll        = null
    retries     = 3
    ratio       = 0.25
    events      = []
    connection  = {}
  }
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{Config: exported},
			{Config: exported, PlanOnly: true},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateIdFunc: importID,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationBadExpressionFailsPlanAtItsAttribute(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config:      config("linear", `$${{ body.tpye == "Issue" }}`, soundThread),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Invalid expression in definition\.webhook\.events\[0\]\.when.*No such key: tpye.*Did you mean body\.type\?`),
			},
			{
				Config:      config("linear", soundWhen, `$${{ task.marks["slack.thread"] + ask.bdoy }}`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Invalid expression in\s+definition\.on\["task\.opened"\]\.to\.thread.*No such key: bdoy.*Did you mean ask\.body\?`),
			},
			{
				Config:      config("linear", `$${{ "Issue" }}`, soundThread),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`must return a bool, not a string`),
			},
		},
	})

	if fake.Count("integration") != 0 {
		t.Fatal("a plan that failed its checks created an integration")
	}
}

func TestIntegrationSkipsExpressionsNotKnownYet(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config: config("linear", soundWhen, "$${{ ask.body }} for ${spotlight_team.core.id}"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("version"), knownvalue.Int64Exact(1)),
				},
			},
		},
	})
}

func TestIntegrationInvalidDefinitionFailsPlan(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config: providertest.Workspace + `
resource "spotlight_integration" "core_linear" {
  team_id = spotlight_team.core.id
  definition = {
    webhook = {
      events = [{ when = "$${{ body.type == \"Issue\" }}" }]
    }
  }
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Invalid definition at definition\.integration.*names the host`),
			},
		},
	})
}
