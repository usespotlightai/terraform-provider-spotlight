package provider_test

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

const secretAddress = "spotlight_secret.linear_api_key"

func secretBuiltIn(name, value string, version int, bindings string) string {
	return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_secret" "linear_api_key" {
  workspace_id     = spotlight_workspace.acme.id
  name             = %q
  bindings         = [%s]
  value_wo         = %q
  value_wo_version = %d
}
`, name, bindings, value, version)
}

const secretLinear = `{ host = "API.linear.app", prefix = "Bearer " }`

const secretLinearAndGitHub = `{ host = "api.linear.app", prefix = "Bearer " }, { host = "api.github.com", header = "x-api-key" }`

func secretImportID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/LINEAR_API_KEY", nil
}

func secretSentValue(fake *fakeapi.Server) (int, bool) {
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

func secretKeeps(fake *fakeapi.Server, want string) resource.TestCheckFunc {
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
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "secret", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: secretBuiltIn("LINEAR_API_KEY", "lin_first", 1, secretLinear),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("address"), knownvalue.StringExact("LINEAR_API_KEY")),
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("value_wo"), knownvalue.Null()),
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("value_wo_version"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("source").AtMapKey("kind"), knownvalue.StringExact("built-in")),
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings").AtSliceIndex(0).AtMapKey("header"), knownvalue.StringExact("authorization")),
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("rotated_at"), knownvalue.NotNull()),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					secretKeeps(fake, "lin_first"),
					acctest.NothingHolds(acctest.Value("lin_first")),
				),
			},
			{
				Config:   secretBuiltIn("LINEAR_API_KEY", "lin_first", 1, secretLinear),
				PlanOnly: true,
			},
			{
				Config:   secretBuiltIn("LINEAR_API_KEY", "lin_changed_but_not_versioned", 1, secretLinear),
				PlanOnly: true,
			},
			{
				Config: secretBuiltIn("LINEAR_API_KEY", "lin_second", 2, secretLinear),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(secretAddress, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					secretKeeps(fake, "lin_second"),
					acctest.NothingHolds(acctest.Value("lin_first"), acctest.Value("lin_second")),
				),
			},
			{
				Config: secretBuiltIn("LINEAR_API_KEY", "lin_second", 2, secretLinearAndGitHub),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(secretAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings"), knownvalue.ListSizeExact(2)),
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("version"), knownvalue.Int64Exact(3)),
				},
				Check: func(*terraform.State) error {
					if sent, last := secretSentValue(fake); sent != 2 || last {
						return fmt.Errorf("the value was sent %d times, the last rebind sending it %v; want twice, not on a rebind", sent, last)
					}

					return nil
				},
			},
			{
				ResourceName:      secretAddress,
				ImportState:       true,
				ImportStateIdFunc: secretImportID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("secret", "LINEAR_API_KEY"), func(fields map[string]any) {
						fields["bindings"] = []any{}
					})
				},
				Config: secretBuiltIn("LINEAR_API_KEY", "lin_second", 2, secretLinearAndGitHub),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(secretAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings"), knownvalue.ListSizeExact(2)),
				},
			},
			{
				Config: secretBuiltIn("LINEAR_TOKEN", "lin_third", 1, secretLinear),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(secretAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: acctest.NothingHolds(acctest.Value("lin_third")),
			},
		},
	})
}

const secretFromAWS = `
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
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + secretFromAWS,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("source").AtMapKey("secret_id"), knownvalue.StringExact("linear/api")),
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("value_wo_version"), knownvalue.Null()),
				},
				Check: secretKeeps(fake, ""),
			},
			{
				Config:   acctest.Workspace + secretFromAWS,
				PlanOnly: true,
			},
			{
				ResourceName:      secretAddress,
				ImportState:       true,
				ImportStateIdFunc: secretImportID,
				ImportStateVerify: true,
			},
			{
				Config:      acctest.Workspace + regexp.MustCompile(`bindings = `).ReplaceAllString(secretFromAWS, "value_wo = \"lin_nope\"\n  bindings = "),
				ExpectError: regexp.MustCompile(`(?s)value_wo: is kept only by Spotlight`),
			},
			{
				Config: secretBuiltIn("LINEAR_API_KEY", "lin_kept_now", 1, secretLinear),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(secretAddress, plancheck.ResourceActionUpdate)},
				},
				Check: secretKeeps(fake, "lin_kept_now"),
			},
		},
	})
}

func TestSecretWritesNeedTheSecretsScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.Grant("configure", "runners") },
				Config:      secretBuiltIn("LINEAR_API_KEY", "lin_first", 1, secretLinear),
				ExpectError: regexp.MustCompile(`(?s)Not allowed to change the secret.*lacks the secrets scope.*configure, secrets and\s+runners`),
			},
		},
	})
}

func secretAccConfig(k acctest.Keys, bindings string, version int) string {
	return fmt.Sprintf(`
variable "secret_value" {
  type      = string
  sensitive = true
}

resource "spotlight_secret" "kept" {
  workspace_id     = local.workspace_id
  name             = %[1]q
  bindings         = [%[3]s]
  value_wo         = var.secret_value
  value_wo_version = %[4]d
}

resource "spotlight_secret" "aws" {
  workspace_id = local.workspace_id
  name         = %[2]q
  source = {
    kind      = "aws-secrets-manager"
    secret_id = "tf-acc/%[2]s"
    field     = "token"
  }
}
`, k.Secret, k.AWSSecret, bindings, version)
}

func secretAccVariables(value string) config.Variables {
	return config.Variables{"secret_value": config.StringVariable(value)}
}

func TestAccSecret(t *testing.T) {
	cp := acctest.PreCheck(t)
	k := acctest.RandomKeys()
	cp.Sweep(t, k)

	first, second := acctest.RandomToken(), acctest.RandomToken()
	id := cp.Workspace.ID
	linear := `{ host = "api.linear.app", prefix = "Bearer " }`
	both := linear + `, { host = "api.github.com", header = "x-api-key" }`

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config:          acctest.AdoptedWorkspace(cp.Workspace) + secretAccConfig(k, linear, 1),
				ConfigVariables: secretAccVariables(first),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_secret.kept", "address", k.Secret),
					resource.TestCheckResourceAttr("spotlight_secret.kept", "source.kind", "built-in"),
					resource.TestCheckResourceAttr("spotlight_secret.kept", "bindings.0.header", "authorization"),
					resource.TestCheckResourceAttr("spotlight_secret.kept", "value_wo_version", "1"),
					resource.TestCheckNoResourceAttr("spotlight_secret.kept", "value_wo"),
					resource.TestCheckResourceAttr("spotlight_secret.aws", "source.secret_id", "tf-acc/"+k.AWSSecret),
					acctest.NothingHolds(acctest.Value(first)),
				),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + secretAccConfig(k, linear, 1),
				ConfigVariables:  secretAccVariables(first),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config:          acctest.AdoptedWorkspace(cp.Workspace) + secretAccConfig(k, both, 2),
				ConfigVariables: secretAccVariables(second),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("spotlight_secret.kept", plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_secret.kept", "bindings.#", "2"),
					resource.TestCheckResourceAttr("spotlight_secret.kept", "value_wo_version", "2"),
					acctest.NothingHolds(acctest.Value(first), acctest.Value(second)),
				),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + secretAccConfig(k, both, 2),
				ConfigVariables:  secretAccVariables(second),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      "spotlight_secret.kept",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Secret),
				ImportStateVerify: true,
				ConfigVariables:   secretAccVariables(second),
			},
			{
				ResourceName:      "spotlight_secret.aws",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.AWSSecret),
				ImportStateVerify: true,
				ConfigVariables:   secretAccVariables(second),
			},
			{
				Config:          acctest.ReleasedWorkspace(cp.Workspace) + secretAccConfig(k, both, 2),
				ConfigVariables: secretAccVariables(second),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					acctest.ExpectForgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction("spotlight_secret.kept", plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}

func TestSecretImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            secretBuiltIn("LINEAR_API_KEY", "lin_first", 1, secretLinearAndGitHub),
				ConfigStateChecks: acctest.ExpectIdentity(secretAddress),
			},
			acctest.ImportedByIdentity(secretAddress),
		},
	})
}
