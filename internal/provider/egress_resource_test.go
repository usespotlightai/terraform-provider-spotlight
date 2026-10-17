package provider_test

import (
	"fmt"
	"regexp"
	"strings"
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

const egressAddress = "spotlight_egress.shared"

func egressAt(key, url, token string, version int) string {
	return fmt.Sprintf(`
resource "spotlight_egress" "shared" {
  workspace_id  = spotlight_workspace.acme.id
  key           = %q
  url           = %q
  token         = %q
  token_version = %d
}
`, key, url, token, version)
}

func egressShared(token string, version int) string {
	return egressAt("shared", "https://egress.example.com", token, version)
}

func egressImportID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/shared", nil
}

func egressKept(fake *fakeapi.Server) string {
	return fake.Kept(fake.IDOf("egress", "shared"))
}

func egressKeeps(fake *fakeapi.Server, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := egressKept(fake); got != want {
			return fmt.Errorf("the egress keeps %q, want %q", got, want)
		}

		return nil
	}
}

func egressToken(body map[string]any) (any, bool) {
	if _, ok := body["url"]; !ok {
		return nil, false
	}

	token, ok := body["token"]

	return token, ok
}

func TestEgressTokenIsWrittenButNeverKept(t *testing.T) {
	fake := fakeapi.New(t)
	updated := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(egressAddress, plancheck.ResourceActionUpdate)},
	}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "egress", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + egressShared(tokenFirst, 1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(egressAddress, tfjsonpath.New("address"), knownvalue.StringExact("shared")),
					statecheck.ExpectKnownValue(egressAddress, tfjsonpath.New("key"), knownvalue.StringExact("shared")),
					statecheck.ExpectKnownValue(egressAddress, tfjsonpath.New("url"), knownvalue.StringExact("https://egress.example.com")),
					statecheck.ExpectKnownValue(egressAddress, tfjsonpath.New("token"), knownvalue.Null()),
					statecheck.ExpectKnownValue(egressAddress, tfjsonpath.New("token_version"), knownvalue.Int64Exact(1)),
				},
				Check: resource.ComposeAggregateTestCheckFunc(egressKeeps(fake, tokenFirst), acctest.NothingContains(acctest.Value(tokenFirst))),
			},
			{
				Config:   acctest.Workspace + egressShared(tokenFirst, 1),
				PlanOnly: true,
			},
			{
				Config:           acctest.Workspace + egressAt("shared", "https://egress.example.com/v2", tokenFirst, 1),
				ConfigPlanChecks: updated,
				Check:            sentTimes(fake, egressToken, 1, "moving the egress sends no token"),
			},
			{
				Config:           acctest.Workspace + egressAt("shared", "https://egress.example.com/v2", tokenSecond, 2),
				ConfigPlanChecks: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					egressKeeps(fake, tokenSecond),
					sentTimes(fake, egressToken, 2, "a new token_version sends the token again"),
					acctest.NothingContains(acctest.Value(tokenFirst), acctest.Value(tokenSecond)),
				),
			},
			{
				ResourceName:      egressAddress,
				ImportState:       true,
				ImportStateIdFunc: egressImportID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("egress", "shared"), func(fields map[string]any) { fields["url"] = "https://elsewhere.example.com" })
				},
				Config:           acctest.Workspace + egressAt("shared", "https://egress.example.com/v2", tokenSecond, 2),
				ConfigPlanChecks: updated,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(egressAddress, tfjsonpath.New("url"), knownvalue.StringExact("https://egress.example.com/v2")),
				},
				Check: sentTimes(fake, egressToken, 2, "putting back what changed sends no token"),
			},
			{
				Config: acctest.Workspace + egressAt("regional", "https://egress.example.com/v2", tokenThird, 3),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(egressAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
			},
		},
	})
}

const egressMinted = `
resource "spotlight_egress" "shared" {
  workspace_id = spotlight_workspace.acme.id
  key          = "shared"
  url          = "https://egress.example.com"
}
`

const egressRotated = `
resource "spotlight_egress" "shared" {
  workspace_id  = spotlight_workspace.acme.id
  key           = "shared"
  url           = "https://egress.example.com"
  token_version = 1
}
`

func TestAnEgressTokenSpotlightIssuesIsNeverKept(t *testing.T) {
	fake := fakeapi.New(t)
	var first string

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + egressMinted,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(egressAddress, tfjsonpath.New("token"), knownvalue.Null()),
					statecheck.ExpectKnownValue(egressAddress, tfjsonpath.New("token_version"), knownvalue.Null()),
				},
				Check: func(state *terraform.State) error {
					first = egressKept(fake)
					if !strings.HasPrefix(first, "issued-") {
						return fmt.Errorf("Spotlight issued no token")
					}

					return acctest.NothingContains(acctest.Value(first))(state)
				},
			},
			{
				Config:   acctest.Workspace + egressMinted,
				PlanOnly: true,
			},
			{
				Config:      acctest.Workspace + egressRotated,
				ExpectError: regexp.MustCompile(`(?s)token: is required whenever tokenVersion changes`),
			},
			{
				Config: acctest.Workspace + egressMinted,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: func(*terraform.State) error {
					if kept := egressKept(fake); kept != first {
						return fmt.Errorf("a rejected rotation changed the egress's token to %q", kept)
					}

					return nil
				},
			},
			{
				Config: acctest.Workspace + egressShared(tokenFirst, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(egressAddress, plancheck.ResourceActionUpdate)},
				},
				Check: egressKeeps(fake, tokenFirst),
			},
		},
	})
}

const egressNeighbour = `
resource "spotlight_egress" "regional" {
  workspace_id  = spotlight_workspace.acme.id
  key           = "regional"
  url           = "https://regional.example.com"
  token         = %q
  token_version = %d
}
`

func egressBeside(token string, version int, neighbour string, neighbourVersion int) string {
	return acctest.Workspace + egressShared(token, version) + fmt.Sprintf(egressNeighbour, neighbour, neighbourVersion)
}

func TestAnEgressTokenIsSingleUse(t *testing.T) {
	fake := fakeapi.New(t)
	alreadyUsed := regexp.MustCompile(`(?s)token: was used before\. Each egress token is single-use`)
	inUse := regexp.MustCompile(`(?s)token: is in use by another egress`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + egressShared(tokenFirst, 1),
			},
			{
				Config: acctest.Workspace + egressShared(tokenSecond, 2),
				Check:  egressKeeps(fake, tokenSecond),
			},
			{
				Config:      acctest.Workspace + egressShared(tokenFirst, 3),
				ExpectError: alreadyUsed,
			},
			{
				Config:      egressBeside(tokenSecond, 2, tokenFirst, 1),
				ExpectError: alreadyUsed,
			},
			{
				Config:      egressBeside(tokenSecond, 2, tokenSecond, 1),
				ExpectError: inUse,
			},
			{
				Config: egressBeside(tokenSecond, 2, tokenThird, 1),
				Check:  egressKeeps(fake, tokenSecond),
			},
			{
				Config: acctest.Workspace + egressShared(tokenSecond, 2),
			},
			{
				Config:      acctest.Workspace + egressShared(tokenThird, 3),
				ExpectError: alreadyUsed,
			},
			{
				Config: acctest.Workspace + egressAt("central", "https://egress.example.com", tokenSecond, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(egressAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				ExpectError: alreadyUsed,
			},
		},
	})
}

func TestSpotlightRejectsWhatAnEgressCannotTake(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      acctest.Workspace + egressShared("short", 1),
				ExpectError: regexp.MustCompile(`(?s)token: must be 32 to 255 URL-safe characters`),
			},
			{
				Config:      acctest.Workspace + egressAt("shared", "ftp://egress.example.com", tokenFirst, 1),
				ExpectError: regexp.MustCompile(`(?s)url: must be an http or https URL`),
			},
			{
				Config:      acctest.Workspace + egressAt("shared", "https://egress.example.com/?region=eu", tokenFirst, 1),
				ExpectError: regexp.MustCompile(`(?s)url: must be at most 2048 characters, with no query and no fragment`),
			},
			{
				Config:      acctest.Workspace + egressAt("Shared Egress", "https://egress.example.com", tokenFirst, 1),
				ExpectError: regexp.MustCompile(`(?s)key: is lowercase words joined by dashes`),
			},
		},
	})
}

func TestAnEgressAPoolNamesIsNotDeleted(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "egress", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: pushPoolThrough(`egress = spotlight_egress.shared.id`),
			},
			{
				Config: acctest.Workspace + `
removed {
  from = spotlight_runner_pool.ephemeral

  lifecycle {
    destroy = false
  }
}
`,
				ExpectError: regexp.MustCompile(`(?s)Egress shared is in use\. Runner pools that name it: ephemeral`),
			},
			{
				PreConfig: func() { fake.Forget(fake.IDOf("runner-pool", "ephemeral")) },
				Config:    acctest.Workspace,
				Check:     acctest.NoneLeft(fake, "egress", "runner-pool"),
			},
		},
	})
}

func TestEgressWritesNeedTheRunnersScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.Grant("configure", "secrets") },
				Config:      acctest.Workspace + egressShared(tokenFirst, 1),
				ExpectError: regexp.MustCompile(`(?s)Not allowed to change the egress.*lacks the runners scope`),
			},
		},
	})
}

func TestEgressImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            acctest.Workspace + egressMinted,
				ConfigStateChecks: acctest.ExpectIdentity(egressAddress),
			},
			acctest.ImportedByIdentity(egressAddress),
		},
	})
}

func egressAccVariables(token, secret string) config.Variables {
	return config.Variables{"egress_token": config.StringVariable(token), "provision_secret": config.StringVariable(secret)}
}

func egressAccConfig(k acctest.Keys, url string, version int) string {
	return fmt.Sprintf(`
variable "egress_token" {
  type      = string
  sensitive = true
}

variable "provision_secret" {
  type      = string
  sensitive = true
}

resource "spotlight_egress" "shared" {
  workspace_id  = local.workspace_id
  key           = %[1]q
  url           = %[3]q
  token         = var.egress_token
  token_version = %[4]d
}

resource "spotlight_runner_pool" "push" {
  workspace_id = local.workspace_id
  key          = %[2]q

  provision = {
    url            = "https://provision.example.com/runs"
    secret         = var.provision_secret
    secret_version = 1
    egress         = spotlight_egress.shared.id
  }
}
`, k.Egress, k.PushPool, url, version)
}

func TestAccEgress(t *testing.T) {
	cp := acctest.PreCheck(t)
	k := acctest.RandomKeys()
	cp.Sweep(t, k)

	firstToken, secondToken := acctest.RandomToken(), acctest.RandomToken()
	secret := acctest.RandomSigningSecret()
	id := cp.Workspace.ID

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config:          acctest.AdoptedWorkspace(cp.Workspace) + egressAccConfig(k, "https://egress.example.com", 1),
				ConfigVariables: egressAccVariables(firstToken, secret),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(egressAddress, "address", k.Egress),
					resource.TestCheckResourceAttr(egressAddress, "url", "https://egress.example.com"),
					resource.TestCheckResourceAttr(egressAddress, "token_version", "1"),
					resource.TestCheckNoResourceAttr(egressAddress, "token"),
					resource.TestCheckResourceAttrPair("spotlight_runner_pool.push", "provision.egress", egressAddress, "id"),
					acctest.NothingContains(acctest.Value(firstToken), acctest.Value(secret)),
				),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + egressAccConfig(k, "https://egress.example.com", 1),
				ConfigVariables:  egressAccVariables(firstToken, secret),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config:          acctest.AdoptedWorkspace(cp.Workspace) + egressAccConfig(k, "https://egress.example.com/v2", 2),
				ConfigVariables: egressAccVariables(secondToken, secret),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(egressAddress, plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("spotlight_runner_pool.push", plancheck.ResourceActionNoop),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(egressAddress, "url", "https://egress.example.com/v2"),
					resource.TestCheckResourceAttr(egressAddress, "token_version", "2"),
					acctest.NothingContains(acctest.Value(firstToken), acctest.Value(secondToken), acctest.Value(secret)),
				),
			},
			{
				ResourceName:      egressAddress,
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Egress),
				ImportStateVerify: true,
				ConfigVariables:   egressAccVariables(secondToken, secret),
			},
			{
				ResourceName:      "spotlight_runner_pool.push",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.PushPool),
				ImportStateVerify: true,
				ConfigVariables:   egressAccVariables(secondToken, secret),
			},
			{
				Config:          acctest.ReleasedWorkspace(cp.Workspace) + egressAccConfig(k, "https://egress.example.com/v2", 2),
				ConfigVariables: egressAccVariables(secondToken, secret),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					acctest.ExpectForgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction(egressAddress, plancheck.ResourceActionNoop),
					plancheck.ExpectResourceAction("spotlight_runner_pool.push", plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}
