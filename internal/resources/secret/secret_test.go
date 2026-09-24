package secret_test

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/providertest"
)

const address = "spotlight_secret.linear_api_key"

var writeOnly = []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)}

func builtIn(name, value string, version int, bindings string) string {
	return providertest.Workspace + fmt.Sprintf(`
resource "spotlight_secret" "linear_api_key" {
  workspace_id     = spotlight_workspace.acme.id
  name             = %q
  bindings         = [%s]
  value_wo         = %q
  value_wo_version = %d
}
`, name, bindings, value, version)
}

const linear = `{ host = "API.linear.app", prefix = "Bearer " }`

const linearAndGitHub = `{ host = "api.linear.app", prefix = "Bearer " }, { host = "api.github.com", header = "x-api-key" }`

func importID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/LINEAR_API_KEY", nil
}

func sentValue(fake *fakeapi.Server) (int, bool) {
	sent, last := 0, false
	for _, each := range fake.Requests() {
		if each.Method != http.MethodPost && each.Method != http.MethodPut {
			continue
		}
		if _, ok := each.Body["name"]; !ok {
			continue
		}
		_, last = each.Body["valueWo"]
		if last {
			sent++
		}
	}

	return sent, last
}

func keeps(fake *fakeapi.Server, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := fake.Kept(fake.IDOf("secret", "LINEAR_API_KEY")); got != want {
			return fmt.Errorf("Spotlight keeps %q, want %q", got, want)
		}

		return nil
	}
}

func TestSecretIsWrittenButItsValueNeverKept(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   writeOnly,
		ProtoV6ProviderFactories: providertest.Factories(fake),
		CheckDestroy:             providertest.NoneLeft(fake, "secret", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: builtIn("LINEAR_API_KEY", "lin_first", 1, linear),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("address"), knownvalue.StringExact("LINEAR_API_KEY")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("value_wo"), knownvalue.Null()),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("value_wo_version"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("source").AtMapKey("kind"), knownvalue.StringExact("built-in")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("bindings").AtSliceIndex(0).AtMapKey("header"), knownvalue.StringExact("authorization")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("rotated_at"), knownvalue.NotNull()),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					keeps(fake, "lin_first"),
					providertest.NothingHolds(providertest.Value("lin_first")),
				),
			},
			{
				Config:   builtIn("LINEAR_API_KEY", "lin_first", 1, linear),
				PlanOnly: true,
			},
			{
				Config:   builtIn("LINEAR_API_KEY", "lin_changed_but_not_versioned", 1, linear),
				PlanOnly: true,
			},
			{
				Config: builtIn("LINEAR_API_KEY", "lin_second", 2, linear),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					keeps(fake, "lin_second"),
					providertest.NothingHolds(providertest.Value("lin_first"), providertest.Value("lin_second")),
				),
			},
			{
				Config: builtIn("LINEAR_API_KEY", "lin_second", 2, linearAndGitHub),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("bindings"), knownvalue.ListSizeExact(2)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("version"), knownvalue.Int64Exact(3)),
				},
				Check: func(*terraform.State) error {
					if sent, last := sentValue(fake); sent != 2 || last {
						return fmt.Errorf("the value was sent %d times, the last rebind sending it %v; want twice, not on a rebind", sent, last)
					}

					return nil
				},
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateIdFunc: importID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("secret", "LINEAR_API_KEY"), func(fields map[string]any) {
						fields["bindings"] = []any{}
					})
				},
				Config: builtIn("LINEAR_API_KEY", "lin_second", 2, linearAndGitHub),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("bindings"), knownvalue.ListSizeExact(2)),
				},
			},
			{
				Config: builtIn("LINEAR_TOKEN", "lin_third", 1, linear),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: providertest.NothingHolds(providertest.Value("lin_third")),
			},
		},
	})
}

const fromAWS = `
resource "spotlight_secret" "linear_api_key" {
  workspace_id = spotlight_workspace.acme.id
  name         = "LINEAR_API_KEY"
  source = {
    kind      = "aws-secrets-manager"
    secret_id = "linear/api"
    field     = "token"
  }
  bindings = [{ host = "api.linear.app" }]
}
`

func TestSecretReadFromAWSTakesNoValue(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   writeOnly,
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config: providertest.Workspace + fromAWS,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("source").AtMapKey("secret_id"), knownvalue.StringExact("linear/api")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("value_wo_version"), knownvalue.Null()),
				},
				Check: keeps(fake, ""),
			},
			{
				Config:   providertest.Workspace + fromAWS,
				PlanOnly: true,
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateIdFunc: importID,
				ImportStateVerify: true,
			},
			{
				Config:      providertest.Workspace + regexp.MustCompile(`bindings = `).ReplaceAllString(fromAWS, "value_wo = \"lin_nope\"\n  bindings = "),
				ExpectError: regexp.MustCompile(`(?s)value_wo: is kept only by Spotlight`),
			},
			{
				Config: builtIn("LINEAR_API_KEY", "lin_kept_now", 1, linear),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				Check: keeps(fake, "lin_kept_now"),
			},
		},
	})
}

func TestSecretWritesNeedTheSecretsScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   writeOnly,
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.Grant("configure", "runners") },
				Config:      builtIn("LINEAR_API_KEY", "lin_first", 1, linear),
				ExpectError: regexp.MustCompile(`(?s)Not allowed to change the secret.*lacks the secrets scope.*configure, secrets and\s+runners`),
			},
		},
	})
}
