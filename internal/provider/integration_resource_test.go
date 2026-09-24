package provider_test

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

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

const integrationAddress = "spotlight_integration.core_linear"

func integrationConfig(host, when, thread string) string {
	return acctest.Workspace + fmt.Sprintf(`
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
	integrationSoundWhen   = `$${{ body.type == "Issue" }}`
	integrationSoundThread = `$${{ ask.body }}`
)

func integrationImportID(state *terraform.State) (string, error) {
	workspace := state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID

	return workspace + "/core/linear", nil
}

func TestIntegrationLifecycle(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "integration", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: integrationConfig("linear", integrationSoundWhen, integrationSoundThread),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(integrationAddress, tfjsonpath.New("source"), knownvalue.StringExact("linear")),
					statecheck.ExpectKnownValue(integrationAddress, tfjsonpath.New("address"), knownvalue.StringExact("core/linear")),
					statecheck.ExpectKnownValue(integrationAddress,
						tfjsonpath.New("definition").AtMapKey("webhook").AtMapKey("events").AtSliceIndex(0).AtMapKey("when"),
						knownvalue.StringExact(`${{ body.type == "Issue" }}`)),
				},
			},
			{
				Config:   integrationConfig("linear", integrationSoundWhen, integrationSoundThread),
				PlanOnly: true,
			},
			{
				ResourceName:            integrationAddress,
				ImportState:             true,
				ImportStateIdFunc:       integrationImportID,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"definition"},
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("integration", "core/linear"), func(fields map[string]any) {
						definition, ok := fields["definition"].(map[string]any)
						if !ok {
							t.Fatalf("the fake holds no definition: %v", fields)
						}
						definition["on"] = map[string]any{}
					})
				},
				Config: integrationConfig("linear", integrationSoundWhen, integrationSoundThread),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(integrationAddress, plancheck.ResourceActionUpdate)},
				},
			},
			{
				Config: integrationConfig("jira", integrationSoundWhen, integrationSoundThread),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(integrationAddress, plancheck.ResourceActionDestroyBeforeCreate),
						plancheck.ExpectKnownValue(integrationAddress, tfjsonpath.New("source"), knownvalue.StringExact("jira")),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(integrationAddress, tfjsonpath.New("address"), knownvalue.StringExact("core/jira")),
				},
			},
		},
	})
}

const integrationExported = `
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

func TestIntegrationImportedAsExportedHasNoDiff(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.Workspace + integrationExported},
			{Config: acctest.Workspace + integrationExported, PlanOnly: true},
			{
				ResourceName:      integrationAddress,
				ImportState:       true,
				ImportStateIdFunc: integrationImportID,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationBadExpressionFailsPlanAtItsAttribute(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      integrationConfig("linear", `$${{ body.tpye == "Issue" }}`, integrationSoundThread),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Invalid expression in definition\.webhook\.events\[0\]\.when.*No such key: tpye.*Did you mean body\.type\?`),
			},
			{
				Config:      integrationConfig("linear", integrationSoundWhen, `$${{ task.marks["slack.thread"] + ask.bdoy }}`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Invalid expression in\s+definition\.on\["task\.opened"\]\.to\.thread.*No such key: bdoy.*Did you mean ask\.body\?`),
			},
			{
				Config:      integrationConfig("linear", `$${{ "Issue" }}`, integrationSoundThread),
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
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: integrationConfig("linear", integrationSoundWhen, "$${{ ask.body }} for ${spotlight_team.core.id}"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(integrationAddress, tfjsonpath.New("version"), knownvalue.Int64Exact(1)),
				},
			},
		},
	})
}

func TestIntegrationInvalidDefinitionFailsPlan(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + `
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

func TestIntegrationImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            acctest.Workspace + integrationExported,
				ConfigStateChecks: acctest.ExpectIdentity(integrationAddress),
			},
			acctest.ImportedByIdentity(integrationAddress),
		},
	})
}
