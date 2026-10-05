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

func integrationImportID(address string) resource.ImportStateIdFunc {
	return func(state *terraform.State) (string, error) {
		workspace := state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID

		return workspace + "/" + address, nil
	}
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
				ImportStateIdFunc:       integrationImportID("core/linear"),
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
				ImportStateIdFunc: integrationImportID("core/linear"),
				ImportStateVerify: true,
			},
		},
	})
}

const integrationActingAsEachPerson = `
resource "spotlight_integration" "core_linear" {
  team_id = spotlight_team.core.id
  definition = {
    integration = "linear"
    connection = {
      holder = "requester"
      oauth2 = {
        authorize    = "https://linear.app/oauth/authorize"
        token        = "https://api.linear.app/oauth/token"
        registration = "https://api.linear.app/oauth/register"
        resource     = "https://mcp.linear.app/mcp"
        scopes       = ["read", "write"]
      }
    }
  }
}
`

func TestIntegrationActingAsEachPersonRoundTrips(t *testing.T) {
	fake := fakeapi.New(t)
	oauth2 := func(key string) tfjsonpath.Path {
		return tfjsonpath.New("definition").AtMapKey("connection").AtMapKey("oauth2").AtMapKey(key)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + integrationActingAsEachPerson,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(integrationAddress,
						tfjsonpath.New("definition").AtMapKey("connection").AtMapKey("holder"),
						knownvalue.StringExact("requester")),
					statecheck.ExpectKnownValue(integrationAddress, oauth2("registration"),
						knownvalue.StringExact("https://api.linear.app/oauth/register")),
					statecheck.ExpectKnownValue(integrationAddress, oauth2("resource"),
						knownvalue.StringExact("https://mcp.linear.app/mcp")),
				},
			},
			{Config: acctest.Workspace + integrationActingAsEachPerson, PlanOnly: true},
			{
				ResourceName:      integrationAddress,
				ImportState:       true,
				ImportStateIdFunc: integrationImportID("core/linear"),
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

const integrationPetstoreAddress = "spotlight_integration.core_petstore"

func integrationPetstoreConfig(description string) string {
	return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_integration" "core_petstore" {
  team_id = spotlight_team.core.id
  definition = {
    integration = "petstore"
    %s
    tools = {
      adopt = { does = "adopt", description = "Adopt a pet." }
    }
  }
}
`, description)
}

func TestIntegrationToolsFromAnAPIDescriptionAreKnownAfterApply(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "integration", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: integrationPetstoreConfig(`openapi = { url = "https://petstore3.swagger.io/api/v3/openapi.json" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue(integrationPetstoreAddress, tfjsonpath.New("tools")),
						plancheck.ExpectUnknownValue(integrationPetstoreAddress, tfjsonpath.New("described")),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(integrationPetstoreAddress, tfjsonpath.New("tools"), knownvalue.MapExact(map[string]knownvalue.Check{
						"adopt":     knownvalue.StringExact("petstore.adopt"),
						"find-pets": knownvalue.StringExact("petstore.find-pets"),
						"add-pet":   knownvalue.StringExact("petstore.add-pet"),
					})),
					statecheck.ExpectKnownValue(integrationPetstoreAddress, tfjsonpath.New("described").AtMapKey("kind"), knownvalue.StringExact("openapi")),
					statecheck.ExpectKnownValue(integrationPetstoreAddress, tfjsonpath.New("described").AtMapKey("tools"), knownvalue.Int64Exact(2)),
					statecheck.ExpectKnownValue(integrationPetstoreAddress, tfjsonpath.New("described").AtMapKey("problem"), knownvalue.Null()),
				},
			},
			{
				Config:   integrationPetstoreConfig(`openapi = { url = "https://petstore3.swagger.io/api/v3/openapi.json" }`),
				PlanOnly: true,
			},
			{
				ResourceName:            integrationPetstoreAddress,
				ImportState:             true,
				ImportStateIdFunc:       integrationImportID("core/petstore"),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"definition"},
			},
			{
				Config: integrationPetstoreConfig(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectKnownValue(integrationPetstoreAddress, tfjsonpath.New("tools"), knownvalue.MapExact(map[string]knownvalue.Check{
							"adopt": knownvalue.StringExact("petstore.adopt"),
						})),
						plancheck.ExpectKnownValue(integrationPetstoreAddress, tfjsonpath.New("described"), knownvalue.Null()),
					},
				},
			},
		},
	})
}

func TestIntegrationToolsWithoutAnAPIDescriptionAreKnownAtPlan(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: integrationPetstoreConfig(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectKnownValue(integrationPetstoreAddress, tfjsonpath.New("tools"), knownvalue.MapExact(map[string]knownvalue.Check{
							"adopt": knownvalue.StringExact("petstore.adopt"),
						})),
						plancheck.ExpectKnownValue(integrationPetstoreAddress, tfjsonpath.New("described"), knownvalue.Null()),
					},
				},
			},
			{
				Config: integrationPetstoreConfig(`graphql = { url = "https://petstore.example.com/graphql" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(integrationPetstoreAddress, plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(integrationPetstoreAddress, tfjsonpath.New("tools")),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(integrationPetstoreAddress, tfjsonpath.New("described").AtMapKey("kind"), knownvalue.StringExact("graphql")),
				},
			},
		},
	})
}

func TestIntegrationToolsOfADefinitionNotKnownYetAreUnknown(t *testing.T) {
	for name, definition := range map[string]string{
		"whole": `spotlight_team.core.id == "" ? local.petstore : local.petstore`,
		"description": `merge(local.petstore, {
    openapi = { url = "https://petstore.example.com/${spotlight_team.core.id}/openapi.json" }
  })`,
	} {
		t.Run(name, func(t *testing.T) {
			fake := fakeapi.New(t)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
				Steps: []resource.TestStep{
					{
						Config: acctest.Workspace + fmt.Sprintf(`
locals {
  petstore = {
    integration = "petstore"
    openapi     = { url = "https://petstore3.swagger.io/api/v3/openapi.json" }
  }
}

resource "spotlight_integration" "core_petstore" {
  team_id    = spotlight_team.core.id
  definition = %s
}
`, definition),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectUnknownValue(integrationPetstoreAddress, tfjsonpath.New("tools")),
							},
						},
						ConfigStateChecks: []statecheck.StateCheck{
							statecheck.ExpectKnownValue(integrationPetstoreAddress, tfjsonpath.New("tools"), knownvalue.MapExact(map[string]knownvalue.Check{
								"find-pets": knownvalue.StringExact("petstore.find-pets"),
								"add-pet":   knownvalue.StringExact("petstore.add-pet"),
							})),
						},
					},
				},
			})
		})
	}
}
