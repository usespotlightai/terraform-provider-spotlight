package provider_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
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

const runnerPoolAddress = "spotlight_runner_pool.ephemeral"

var (
	tokenFirst      = strings.Repeat("a", 40)
	tokenSecond     = strings.Repeat("b", 40)
	tokenThird      = strings.Repeat("c", 40)
	provisionFirst  = "whsec_" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("f", 32)))
	provisionSecond = "whsec_" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 32)))
)

func pullPool(key, labels, token string, version int) string {
	return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_runner_pool" "ephemeral" {
  workspace_id  = spotlight_workspace.acme.id
  key           = %q
  labels        = [%s]
  max_runs      = 4
  token         = %q
  token_version = %d
}
`, key, labels, token, version)
}

func pushPool(url, secret string, version int) string {
	return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_runner_pool" "ephemeral" {
  workspace_id = spotlight_workspace.acme.id
  key          = "ephemeral"

  provision = {
    url            = %q
    secret         = %q
    secret_version = %d
  }
}
`, url, secret, version)
}

func runnerPoolImportID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/ephemeral", nil
}

func runnerPoolKept(fake *fakeapi.Server) string {
	return fake.Kept(fake.IDOf("runner-pool", "ephemeral"))
}

func runnerPoolKeeps(fake *fakeapi.Server, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := runnerPoolKept(fake); got != want {
			return fmt.Errorf("the pool keeps %q, want %q", got, want)
		}

		return nil
	}
}

func runnerPoolSent(fake *fakeapi.Server, field func(map[string]any) (any, bool)) int {
	sent := 0
	for _, each := range fake.Requests() {
		if each.Method == http.MethodPost || each.Method == http.MethodPut {
			if _, ok := field(each.Body); ok {
				sent++
			}
		}
	}

	return sent
}

func poolToken(body map[string]any) (any, bool) {
	token, ok := body["token"]

	return token, ok
}

func provisionSecret(body map[string]any) (any, bool) {
	provision, _ := body["provision"].(map[string]any)
	secret, ok := provision["secret"]

	return secret, ok
}

func sentTimes(fake *fakeapi.Server, field func(map[string]any) (any, bool), want int, why string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if sent := runnerPoolSent(fake, field); sent != want {
			return fmt.Errorf("sent %d times, want %d: %s", sent, want, why)
		}

		return nil
	}
}

func TestRunnerPoolTokenIsWrittenButNeverKept(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "runner-pool", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: pullPool("ephemeral", `"linux"`, tokenFirst, 1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("address"), knownvalue.StringExact("ephemeral")),
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("token"), knownvalue.Null()),
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("token_version"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("max_runs"), knownvalue.Int64Exact(4)),
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("provision"), knownvalue.Null()),
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("labels"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("linux")})),
				},
				Check: resource.ComposeAggregateTestCheckFunc(runnerPoolKeeps(fake, tokenFirst), acctest.NothingContains(acctest.Value(tokenFirst))),
			},
			{
				Config:   pullPool("ephemeral", `"linux"`, tokenFirst, 1),
				PlanOnly: true,
			},
			{
				Config: pullPool("ephemeral", `"linux", "gpu"`, tokenFirst, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerPoolAddress, plancheck.ResourceActionUpdate)},
				},
				Check: sentTimes(fake, poolToken, 1, "relabelling sends no token"),
			},
			{
				Config: pullPool("ephemeral", `"gpu", "linux"`, tokenSecond, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerPoolAddress, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					runnerPoolKeeps(fake, tokenSecond),
					sentTimes(fake, poolToken, 2, "a new token_version sends the token again"),
					acctest.NothingContains(acctest.Value(tokenFirst), acctest.Value(tokenSecond)),
				),
			},
			{
				ResourceName:      runnerPoolAddress,
				ImportState:       true,
				ImportStateIdFunc: runnerPoolImportID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("runner-pool", "ephemeral"), func(fields map[string]any) { fields["maxRuns"] = nil })
				},
				Config: pullPool("ephemeral", `"gpu", "linux"`, tokenSecond, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerPoolAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("max_runs"), knownvalue.Int64Exact(4)),
				},
				Check: sentTimes(fake, poolToken, 2, "putting back what changed sends no token"),
			},
			{
				Config: pullPool("burst", `"gpu"`, tokenThird, 3),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerPoolAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
			},
		},
	})
}

const runnerPoolMinted = `
resource "spotlight_runner_pool" "ephemeral" {
  workspace_id = spotlight_workspace.acme.id
  key          = "ephemeral"
}
`

const runnerPoolRotated = `
resource "spotlight_runner_pool" "ephemeral" {
  workspace_id  = spotlight_workspace.acme.id
  key           = "ephemeral"
  token_version = 1
}
`

func runnerPoolKeepsIssued(fake *fakeapi.Server, first *string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if kept := runnerPoolKept(fake); kept != *first {
			return fmt.Errorf("a rejected rotation changed the pool's token to %q", kept)
		}

		return nil
	}
}

func TestAPoolTokenSpotlightIssuesIsNeverKept(t *testing.T) {
	fake := fakeapi.New(t)
	var first string

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + runnerPoolMinted,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("token"), knownvalue.Null()),
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("token_version"), knownvalue.Null()),
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("max_runs"), knownvalue.Null()),
				},
				Check: func(state *terraform.State) error {
					first = runnerPoolKept(fake)
					if !strings.HasPrefix(first, "issued-") {
						return fmt.Errorf("Spotlight issued no token")
					}

					return acctest.NothingContains(acctest.Value(first))(state)
				},
			},
			{
				Config:   acctest.Workspace + runnerPoolMinted,
				PlanOnly: true,
			},
			{
				Config:      acctest.Workspace + runnerPoolRotated,
				ExpectError: regexp.MustCompile(`(?s)token: is given whenever tokenVersion changes`),
			},
			{
				Config: acctest.Workspace + runnerPoolMinted,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: runnerPoolKeepsIssued(fake, &first),
			},
			{
				Config: pullPool("ephemeral", "", tokenFirst, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerPoolAddress, plancheck.ResourceActionUpdate)},
				},
				Check: runnerPoolKeeps(fake, tokenFirst),
			},
		},
	})
}

func TestAPushPoolSecretIsWrittenButNeverKept(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "runner-pool", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: pushPool("https://provision.example.com/runs", provisionFirst, 1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("token"), knownvalue.Null()),
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("token_version"), knownvalue.Null()),
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("provision"), knownvalue.ObjectExact(map[string]knownvalue.Check{
						"url":            knownvalue.StringExact("https://provision.example.com/runs"),
						"secret":         knownvalue.Null(),
						"secret_version": knownvalue.Int64Exact(1),
						"egress":         knownvalue.Null(),
						"egress_traffic": knownvalue.StringExact("all"),
					})),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					runnerPoolKeeps(fake, provisionFirst),
					sentTimes(fake, poolToken, 0, "a pool that provisions has no token"),
					acctest.NothingContains(acctest.Value(provisionFirst)),
				),
			},
			{
				Config:   pushPool("https://provision.example.com/runs", provisionFirst, 1),
				PlanOnly: true,
			},
			{
				Config: pushPool("https://provision.example.com/v2/runs", provisionFirst, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerPoolAddress, plancheck.ResourceActionUpdate)},
				},
				Check: sentTimes(fake, provisionSecret, 1, "moving where the pool provisions sends no secret"),
			},
			{
				Config: pushPool("https://provision.example.com/v2/runs", provisionSecond, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerPoolAddress, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					runnerPoolKeeps(fake, provisionSecond),
					sentTimes(fake, provisionSecret, 2, "a new secret_version sends the secret again"),
					acctest.NothingContains(acctest.Value(provisionFirst), acctest.Value(provisionSecond)),
				),
			},
			{
				ResourceName:      runnerPoolAddress,
				ImportState:       true,
				ImportStateIdFunc: runnerPoolImportID,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAddingOrRemovingProvisionReplacesThePool(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: pullPool("ephemeral", "", tokenFirst, 1),
			},
			{
				Config: pushPool("https://provision.example.com/runs", provisionFirst, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerPoolAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: runnerPoolKeeps(fake, provisionFirst),
			},
			{
				Config: pullPool("ephemeral", "", tokenSecond, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerPoolAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: runnerPoolKeeps(fake, tokenSecond),
			},
		},
	})
}

const runnerPoolNeighbour = `
resource "spotlight_runner_pool" "burst" {
  workspace_id  = spotlight_workspace.acme.id
  key           = "burst"
  token         = %q
  token_version = %d
}
`

func pullPoolBeside(token string, version int, neighbour string, neighbourVersion int) string {
	return pullPool("ephemeral", "", token, version) + fmt.Sprintf(runnerPoolNeighbour, neighbour, neighbourVersion)
}

func TestAPoolTokenIsGivenOnceEver(t *testing.T) {
	fake := fakeapi.New(t)
	spent := regexp.MustCompile(`(?s)token: was given to a pool before, and a pool token is given once, ever`)
	taken := regexp.MustCompile(`(?s)token: is already another pool’s token`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: pullPool("ephemeral", "", tokenFirst, 1),
			},
			{
				Config: pullPool("ephemeral", "", tokenSecond, 2),
				Check:  runnerPoolKeeps(fake, tokenSecond),
			},
			{
				Config:      pullPool("ephemeral", "", tokenFirst, 3),
				ExpectError: spent,
			},
			{
				Config:      pullPoolBeside(tokenSecond, 2, tokenFirst, 1),
				ExpectError: spent,
			},
			{
				Config:      pullPoolBeside(tokenSecond, 2, tokenSecond, 1),
				ExpectError: taken,
			},
			{
				Config: pullPoolBeside(tokenSecond, 2, tokenThird, 1),
				Check:  runnerPoolKeeps(fake, tokenSecond),
			},
			{
				Config: pullPool("ephemeral", "", tokenSecond, 2),
			},
			{
				Config:      pullPool("ephemeral", "", tokenThird, 3),
				ExpectError: spent,
			},
			{
				Config: pullPool("sandboxes", "", tokenSecond, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerPoolAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				ExpectError: spent,
			},
		},
	})
}

func TestSpotlightRejectsWhatAPoolCannotTake(t *testing.T) {
	fake := fakeapi.New(t)

	both := acctest.Workspace + `
resource "spotlight_runner_pool" "ephemeral" {
  workspace_id  = spotlight_workspace.acme.id
  key           = "ephemeral"
  token         = "` + tokenFirst + `"
  token_version = 1

  provision = {
    url    = "https://provision.example.com/runs"
    secret = "` + provisionFirst + `"
  }
}
`
	unsigned := acctest.Workspace + `
resource "spotlight_runner_pool" "ephemeral" {
  workspace_id = spotlight_workspace.acme.id
  key          = "ephemeral"

  provision = {
    url = "https://provision.example.com/runs"
  }
}
`
	unlimited := strings.Replace(pullPool("ephemeral", "", tokenFirst, 1), "max_runs      = 4", "max_runs      = 0", 1)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      both,
				ExpectError: regexp.MustCompile(`(?s)A pool that provisions takes no token.*token: is only for a pool that is pulled from.*token_version: is only for a pool`),
			},
			{
				Config:      unsigned,
				ExpectError: regexp.MustCompile(`(?s)attribute "secret" is\s+required`),
			},
			{
				Config:      pushPool("https://provision.example.com/runs", "whsec_short", 1),
				ExpectError: regexp.MustCompile(`(?s)provision\.secret: is whsec_ followed by 24 to 64 base64-encoded random bytes`),
			},
			{
				Config:      pullPool("ephemeral", "", "short", 1),
				ExpectError: regexp.MustCompile(`(?s)token: is 32 to 255 URL-safe characters`),
			},
			{
				Config:      unlimited,
				ExpectError: regexp.MustCompile(`(?s)max_runs: is a whole number from 1 to 10000`),
			},
		},
	})
}

func TestRunnerPoolWritesNeedTheRunnersScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.Grant("configure", "secrets") },
				Config:      pullPool("ephemeral", "", tokenFirst, 1),
				ExpectError: regexp.MustCompile(`(?s)Not allowed to change the runner-pool.*lacks the runners scope`),
			},
		},
	})
}

func TestRunnerPoolImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            acctest.Workspace + runnerPoolMinted,
				ConfigStateChecks: acctest.ExpectIdentity(runnerPoolAddress),
			},
			acctest.ImportedByIdentity(runnerPoolAddress),
		},
	})
}

func runnerPoolAccVariables(token, secret string) config.Variables {
	return config.Variables{"pool_token": config.StringVariable(token), "provision_secret": config.StringVariable(secret)}
}

func runnerPoolAccConfig(k acctest.Keys, labels string, version int) string {
	return fmt.Sprintf(`
variable "pool_token" {
  type      = string
  sensitive = true
}

variable "provision_secret" {
  type      = string
  sensitive = true
}

resource "spotlight_runner_pool" "pull" {
  workspace_id  = local.workspace_id
  key           = %[1]q
  labels        = [%[3]s]
  max_runs      = 2
  token         = var.pool_token
  token_version = %[4]d
}

resource "spotlight_runner_pool" "push" {
  workspace_id = local.workspace_id
  key          = %[2]q
  labels       = [%[3]s]

  provision = {
    url            = "https://provision.example.com/runs"
    secret         = var.provision_secret
    secret_version = %[4]d
  }
}
`, k.Pool, k.PushPool, labels, version)
}

func TestAccRunnerPool(t *testing.T) {
	cp := acctest.PreCheck(t)
	k := acctest.RandomKeys()
	cp.Sweep(t, k)

	firstToken, secondToken := acctest.RandomToken(), acctest.RandomToken()
	firstSecret, secondSecret := acctest.RandomSigningSecret(), acctest.RandomSigningSecret()
	id := cp.Workspace.ID

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config:          acctest.AdoptedWorkspace(cp.Workspace) + runnerPoolAccConfig(k, `"tf-acc-linux"`, 1),
				ConfigVariables: runnerPoolAccVariables(firstToken, firstSecret),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_runner_pool.pull", "address", k.Pool),
					resource.TestCheckResourceAttr("spotlight_runner_pool.pull", "max_runs", "2"),
					resource.TestCheckResourceAttr("spotlight_runner_pool.pull", "token_version", "1"),
					resource.TestCheckNoResourceAttr("spotlight_runner_pool.pull", "token"),
					resource.TestCheckNoResourceAttr("spotlight_runner_pool.pull", "provision"),
					resource.TestCheckResourceAttr("spotlight_runner_pool.push", "provision.url", "https://provision.example.com/runs"),
					resource.TestCheckResourceAttr("spotlight_runner_pool.push", "provision.secret_version", "1"),
					resource.TestCheckNoResourceAttr("spotlight_runner_pool.push", "provision.secret"),
					resource.TestCheckNoResourceAttr("spotlight_runner_pool.push", "token_version"),
					acctest.NothingContains(acctest.Value(firstToken), acctest.Value(firstSecret)),
				),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + runnerPoolAccConfig(k, `"tf-acc-linux"`, 1),
				ConfigVariables:  runnerPoolAccVariables(firstToken, firstSecret),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config:          acctest.AdoptedWorkspace(cp.Workspace) + runnerPoolAccConfig(k, `"tf-acc-linux", "tf-acc-gpu"`, 2),
				ConfigVariables: runnerPoolAccVariables(secondToken, secondSecret),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("spotlight_runner_pool.pull", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("spotlight_runner_pool.push", plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_runner_pool.pull", "labels.#", "2"),
					resource.TestCheckResourceAttr("spotlight_runner_pool.push", "provision.secret_version", "2"),
					acctest.NothingContains(acctest.Value(firstToken), acctest.Value(secondToken), acctest.Value(firstSecret), acctest.Value(secondSecret)),
				),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + runnerPoolAccConfig(k, `"tf-acc-gpu", "tf-acc-linux"`, 2),
				ConfigVariables:  runnerPoolAccVariables(secondToken, secondSecret),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      "spotlight_runner_pool.pull",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Pool),
				ImportStateVerify: true,
				ConfigVariables:   runnerPoolAccVariables(secondToken, secondSecret),
			},
			{
				ResourceName:      "spotlight_runner_pool.push",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.PushPool),
				ImportStateVerify: true,
				ConfigVariables:   runnerPoolAccVariables(secondToken, secondSecret),
			},
			{
				Config:          acctest.ReleasedWorkspace(cp.Workspace) + runnerPoolAccConfig(k, `"tf-acc-gpu", "tf-acc-linux"`, 2),
				ConfigVariables: runnerPoolAccVariables(secondToken, secondSecret),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					acctest.ExpectForgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction("spotlight_runner_pool.pull", plancheck.ResourceActionNoop),
					plancheck.ExpectResourceAction("spotlight_runner_pool.push", plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}

func pushPoolThrough(egress string) string {
	return acctest.Workspace + egressShared(tokenFirst, 1) + fmt.Sprintf(`
resource "spotlight_runner_pool" "ephemeral" {
  workspace_id = spotlight_workspace.acme.id
  key          = "ephemeral"

  provision = {
    url            = "https://provision.example.com/runs"
    secret         = %q
    secret_version = 1
    %s
  }
}
`, provisionFirst, egress)
}

func TestAPushPoolNamesItsEgress(t *testing.T) {
	fake := fakeapi.New(t)
	updated := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerPoolAddress, plancheck.ResourceActionUpdate)},
	}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "runner-pool", "egress", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: pushPoolThrough(`egress = "spotlight"`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("provision").AtMapKey("egress"), knownvalue.StringExact("spotlight")),
				},
			},
			{
				Config:           pushPoolThrough(`egress = spotlight_egress.shared.id`),
				ConfigPlanChecks: updated,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs(runnerPoolAddress, tfjsonpath.New("provision").AtMapKey("egress"), egressAddress, tfjsonpath.New("id"), compare.ValuesSame()),
				},
				Check: sentTimes(fake, provisionSecret, 1, "naming another egress sends no secret"),
			},
			{
				Config:   pushPoolThrough(`egress = spotlight_egress.shared.id`),
				PlanOnly: true,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("provision").AtMapKey("egress_traffic"), knownvalue.StringExact("all")),
				},
			},
			{
				Config:           pushPoolThrough("egress = spotlight_egress.shared.id\n    egress_traffic = \"bound\""),
				ConfigPlanChecks: updated,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("provision").AtMapKey("egress_traffic"), knownvalue.StringExact("bound")),
				},
			},
			{
				Config:      pushPoolThrough("egress = spotlight_egress.shared.id\n    egress_traffic = \"some\""),
				ExpectError: regexp.MustCompile(`(?s)egress_traffic.*value must be one of`),
			},
			{
				Config:           pushPoolThrough(`egress = spotlight_egress.shared.id`),
				ConfigPlanChecks: updated,
			},
			{
				ResourceName:      runnerPoolAddress,
				ImportState:       true,
				ImportStateIdFunc: runnerPoolImportID,
				ImportStateVerify: true,
			},
			{
				Config:      pushPoolThrough(`egress = "egress-9999"`),
				ExpectError: regexp.MustCompile(`(?s)provision\.egress: names egress-9999\. Egress egress-9999 doesn't exist in this\s+workspace`),
			},
			{
				Config:           pushPoolThrough(""),
				ConfigPlanChecks: updated,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerPoolAddress, tfjsonpath.New("provision").AtMapKey("egress"), knownvalue.Null()),
				},
			},
			{
				Config: pushPool("https://provision.example.com/runs", provisionFirst, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(runnerPoolAddress, plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction(egressAddress, plancheck.ResourceActionDestroy),
					},
				},
				Check: acctest.NoneLeft(fake, "egress"),
			},
		},
	})
}
