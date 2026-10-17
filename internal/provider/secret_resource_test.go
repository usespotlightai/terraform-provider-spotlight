package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
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

const secretLinearAndGitHub = `{ host = "api.linear.app", prefix = "Bearer " }, { host = "api.github.com", name = "x-api-key" }`

const secretMaps = `{ host = "maps.googleapis.com", location = "query", name = "key" }`

func secretImportID(state *terraform.State) (string, error) {
	return secretImportIDOf("LINEAR_API_KEY")(state)
}

func secretImportIDOf(name string) resource.ImportStateIdFunc {
	return func(state *terraform.State) (string, error) {
		return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/" + name, nil
	}
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
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings").AtSliceIndex(0).AtMapKey("location"), knownvalue.StringExact("header")),
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings").AtSliceIndex(0).AtMapKey("name"), knownvalue.StringExact("authorization")),
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("rotated_at"), knownvalue.NotNull()),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					secretKeeps(fake, "lin_first"),
					acctest.NothingContains(acctest.Value("lin_first")),
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
					acctest.NothingContains(acctest.Value("lin_first"), acctest.Value("lin_second")),
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
				Check: acctest.NothingContains(acctest.Value("lin_third")),
			},
		},
	})
}

func TestSecretIsInjectedInAQueryParameter(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      secretBuiltIn("GOOGLE_MAPS_KEY", "maps_first", 1, `{ host = "maps.googleapis.com", location = "query" }`),
				ExpectError: regexp.MustCompile(`(?s)A query parameter must be named`),
			},
			{
				Config:      secretBuiltIn("GOOGLE_MAPS_KEY", "maps_first", 1, `{ host = "maps.googleapis.com", location = "cookie", name = "key" }`),
				ExpectError: regexp.MustCompile(`(?s)value must be one of`),
			},
			{
				Config: secretBuiltIn("GOOGLE_MAPS_KEY", "maps_first", 1, secretMaps),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings").AtSliceIndex(0).AtMapKey("location"), knownvalue.StringExact("query")),
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings").AtSliceIndex(0).AtMapKey("name"), knownvalue.StringExact("key")),
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings").AtSliceIndex(0).AtMapKey("prefix"), knownvalue.StringExact("")),
				},
			},
			{
				Config:           secretBuiltIn("GOOGLE_MAPS_KEY", "maps_first", 1, secretMaps),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      secretAddress,
				ImportState:       true,
				ImportStateIdFunc: secretImportIDOf("GOOGLE_MAPS_KEY"),
				ImportStateVerify: true,
			},
			{
				Config: secretBuiltIn("GOOGLE_MAPS_KEY", "maps_first", 1, `{ host = "maps.googleapis.com" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(secretAddress, plancheck.ResourceActionUpdate),
					plancheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings").AtSliceIndex(0).AtMapKey("name"), knownvalue.StringExact("authorization")),
				}},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings").AtSliceIndex(0).AtMapKey("location"), knownvalue.StringExact("header")),
				},
			},
			{
				Config:           secretBuiltIn("GOOGLE_MAPS_KEY", "maps_first", 1, `{ host = "maps.googleapis.com" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

const secretMixedCaseHeader = `{ host = "api.example.com", name = "X-Api-Key" }`

func TestSecretKeepsAHeaderNameAsWritten(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: secretBuiltIn("EXAMPLE_API_KEY", "ex_first", 1, secretMixedCaseHeader),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings").AtSliceIndex(0).AtMapKey("name"), knownvalue.StringExact("X-Api-Key")),
				},
			},
			{
				Config:           secretBuiltIn("EXAMPLE_API_KEY", "ex_first", 1, secretMixedCaseHeader),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      secretAddress,
				ImportState:       true,
				ImportStateIdFunc: secretImportIDOf("EXAMPLE_API_KEY"),
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if got := states[0].Attributes["bindings.0.name"]; got != "x-api-key" {
						return fmt.Errorf("Spotlight stores the header as %q, want x-api-key", got)
					}

					return nil
				},
			},
		},
	})
}

const secretBindingsNotYetKnown = `
resource "terraform_data" "location" {
  input = "query"
}

resource "terraform_data" "parameter" {
  input = "key"
}

resource "spotlight_secret" "linear_api_key" {
  workspace_id     = spotlight_workspace.acme.id
  name             = "GOOGLE_MAPS_KEY"
  bindings         = [{ host = "maps.googleapis.com", location = terraform_data.location.output, name = "key" }, { host = "api.example.com", location = "query", name = terraform_data.parameter.output }]
  value_wo         = "maps_first"
  value_wo_version = 1
}
`

func TestSecretBindingsNotYetKnownAreLeftForThePlan(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + secretBindingsNotYetKnown,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings"), knownvalue.ListSizeExact(2)),
					statecheck.ExpectKnownValue(secretAddress, tfjsonpath.New("bindings").AtSliceIndex(1).AtMapKey("name"), knownvalue.StringExact("key")),
				},
			},
		},
	})
}

func TestSecretValidatesABindingNotYetKnown(t *testing.T) {
	ctx := context.Background()
	secret := &provider.SecretResource{}

	var described fwresource.SchemaResponse
	secret.Schema(ctx, fwresource.SchemaRequest{}, &described)

	whole, ok := described.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("the secret's schema is not an object")
	}
	bindings, ok := whole.AttributeTypes["bindings"].(tftypes.List)
	if !ok {
		t.Fatal("bindings is not a list")
	}
	binding := bindings.ElementType
	values := map[string]tftypes.Value{}
	for name, typ := range whole.AttributeTypes {
		values[name] = tftypes.NewValue(typ, nil)
	}
	values["bindings"] = tftypes.NewValue(tftypes.List{ElementType: binding}, []tftypes.Value{tftypes.NewValue(binding, tftypes.UnknownValue)})

	var validated fwresource.ValidateConfigResponse
	secret.ValidateConfig(ctx, fwresource.ValidateConfigRequest{Config: tfsdk.Config{
		Schema: described.Schema,
		Raw:    tftypes.NewValue(whole, values),
	}}, &validated)

	if validated.Diagnostics.HasError() {
		t.Fatalf("a binding not yet known was refused: %v", validated.Diagnostics)
	}
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
				ExpectError: regexp.MustCompile(`(?s)Not allowed to change the secret.*lacks the secrets scope.*configure, secrets,\s+runners\s+and\s+trusts`),
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
	both := linear + `, { host = "api.github.com", name = "x-api-key" }`

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
					resource.TestCheckResourceAttr("spotlight_secret.kept", "bindings.0.location", "header"),
					resource.TestCheckResourceAttr("spotlight_secret.kept", "bindings.0.name", "authorization"),
					resource.TestCheckResourceAttr("spotlight_secret.kept", "value_wo_version", "1"),
					resource.TestCheckNoResourceAttr("spotlight_secret.kept", "value_wo"),
					resource.TestCheckResourceAttr("spotlight_secret.aws", "source.secret_id", "tf-acc/"+k.AWSSecret),
					acctest.NothingContains(acctest.Value(first)),
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
					acctest.NothingContains(acctest.Value(first), acctest.Value(second)),
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
