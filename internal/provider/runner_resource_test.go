package provider_test

import (
	"fmt"
	"net/http"
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

const runnerAddress = "spotlight_runner.build"

var (
	runnerFirst  = strings.Repeat("a", 40)
	runnerSecond = strings.Repeat("b", 40)
)

func runnerWithToken(name, labels, token string, version int) string {
	return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_runner" "build" {
  workspace_id  = spotlight_workspace.acme.id
  name          = %q
  labels        = [%s]
  token         = %q
  token_version = %d
}
`, name, labels, token, version)
}

func runnerImportID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/build-1", nil
}

func runnerKeeps(fake *fakeapi.Server, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := fake.Kept(fake.IDOf("runner", "build-1")); got != want {
			return fmt.Errorf("the runner calls with %q, want %q", got, want)
		}

		return nil
	}
}

func runnerTokensSent(fake *fakeapi.Server) int {
	sent := 0
	for _, each := range fake.Requests() {
		if each.Method == http.MethodPost || each.Method == http.MethodPut {
			if _, ok := each.Body["token"]; ok {
				sent++
			}
		}
	}

	return sent
}

func TestRunnerTokenIsWrittenButNeverKept(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "runner", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: runnerWithToken("build-1", `"vpc-internal"`, runnerFirst, 1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerAddress, tfjsonpath.New("address"), knownvalue.StringExact("build-1")),
					statecheck.ExpectKnownValue(runnerAddress, tfjsonpath.New("token"), knownvalue.Null()),
					statecheck.ExpectKnownValue(runnerAddress, tfjsonpath.New("trust"), knownvalue.Null()),
					statecheck.ExpectKnownValue(runnerAddress, tfjsonpath.New("labels"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("vpc-internal")})),
				},
				Check: resource.ComposeAggregateTestCheckFunc(runnerKeeps(fake, runnerFirst), acctest.NothingHolds(acctest.Value(runnerFirst))),
			},
			{
				Config:   runnerWithToken("build-1", `"vpc-internal"`, runnerFirst, 1),
				PlanOnly: true,
			},
			{
				Config: runnerWithToken("build-1", `"vpc-internal", "gpu"`, runnerFirst, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerAddress, plancheck.ResourceActionUpdate)},
				},
				Check: func(*terraform.State) error {
					if sent := runnerTokensSent(fake); sent != 1 {
						return fmt.Errorf("the token was sent %d times, want once: relabelling sends none", sent)
					}

					return nil
				},
			},
			{
				Config: runnerWithToken("build-1", `"gpu", "vpc-internal"`, runnerSecond, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerAddress, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(runnerKeeps(fake, runnerSecond), acctest.NothingHolds(acctest.Value(runnerFirst), acctest.Value(runnerSecond))),
			},
			{
				ResourceName:            runnerAddress,
				ImportState:             true,
				ImportStateIdFunc:       runnerImportID,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"token_version"},
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("runner", "build-1"), func(fields map[string]any) { fields["labels"] = []any{} })
				},
				Config: runnerWithToken("build-1", `"gpu", "vpc-internal"`, runnerSecond, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerAddress, tfjsonpath.New("labels"), knownvalue.SetSizeExact(2)),
				},
			},
			{
				Config: runnerWithToken("build-2", `"gpu"`, runnerSecond, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
			},
		},
	})
}

const runnerMinted = `
resource "spotlight_runner" "build" {
  workspace_id = spotlight_workspace.acme.id
  name         = "build-1"
}
`

func TestARunnerTokenSpotlightIssuesIsNeverKept(t *testing.T) {
	fake := fakeapi.New(t)
	issued := func() string { return fake.Kept(fake.IDOf("runner", "build-1")) }

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + runnerMinted,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerAddress, tfjsonpath.New("token"), knownvalue.Null()),
					statecheck.ExpectKnownValue(runnerAddress, tfjsonpath.New("labels"), knownvalue.SetSizeExact(0)),
				},
				Check: func(state *terraform.State) error {
					if !strings.HasPrefix(issued(), "issued-") {
						return fmt.Errorf("Spotlight issued no token")
					}

					return acctest.NothingHolds(issued)(state)
				},
			},
			{
				Config:   acctest.Workspace + runnerMinted,
				PlanOnly: true,
			},
			{
				Config: runnerWithToken("build-1", "", runnerFirst, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerAddress, plancheck.ResourceActionUpdate)},
				},
				Check: runnerKeeps(fake, runnerFirst),
			},
		},
	})
}

func TestRunnerWritesNeedTheRunnersScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.Grant("configure", "secrets") },
				Config:      runnerWithToken("build-1", "", runnerFirst, 1),
				ExpectError: regexp.MustCompile(`(?s)Not allowed to change the runner.*lacks the runners scope`),
			},
		},
	})
}

func TestARunnerTokenMustBeLongEnough(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      runnerWithToken("build-1", "", "short", 1),
				ExpectError: regexp.MustCompile(`(?s)token: is 32 to 255 URL-safe characters`),
			},
		},
	})
}

func runnerAccVariables(token string) config.Variables {
	return config.Variables{"runner_token": config.StringVariable(token)}
}

func runnerAccConfig(k acctest.Keys, labels string, version int) string {
	return fmt.Sprintf(`
variable "runner_token" {
  type      = string
  sensitive = true
}

resource "spotlight_runner" "build" {
  workspace_id  = local.workspace_id
  name          = %[1]q
  labels        = [%[3]s]
  token         = var.runner_token
  token_version = %[4]d
}

resource "spotlight_runner" "issued" {
  workspace_id = local.workspace_id
  name         = %[2]q
}

resource "spotlight_runner_trust" "cluster" {
  workspace_id = local.workspace_id
  key          = %[5]q
  issuer       = "https://oidc.eks.eu-west-1.amazonaws.com/id/TFACC"
  subject      = "system:serviceaccount:tf-acc:%[5]s"
  labels       = [%[3]s]
}
`, k.Runner, k.IssuedRunner, labels, version, k.Trust)
}

func TestAccRunnerAndRunnerTrust(t *testing.T) {
	cp := acctest.PreCheck(t)
	k := acctest.RandomKeys()
	cp.Sweep(t, k)

	first, second := acctest.RandomToken(), acctest.RandomToken()
	id := cp.Workspace.ID

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config:          acctest.AdoptedWorkspace(cp.Workspace) + runnerAccConfig(k, `"tf-acc-gpu"`, 1),
				ConfigVariables: runnerAccVariables(first),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_runner.build", "address", k.Runner),
					resource.TestCheckResourceAttr("spotlight_runner.build", "labels.#", "1"),
					resource.TestCheckNoResourceAttr("spotlight_runner.build", "token"),
					resource.TestCheckNoResourceAttr("spotlight_runner.build", "trust"),
					resource.TestCheckNoResourceAttr("spotlight_runner.issued", "token"),
					resource.TestCheckResourceAttr("spotlight_runner_trust.cluster", "audience", "spotlight"),
					acctest.NothingHolds(acctest.Value(first)),
				),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + runnerAccConfig(k, `"tf-acc-gpu"`, 1),
				ConfigVariables:  runnerAccVariables(first),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config:          acctest.AdoptedWorkspace(cp.Workspace) + runnerAccConfig(k, `"tf-acc-gpu", "tf-acc-vpc"`, 2),
				ConfigVariables: runnerAccVariables(second),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("spotlight_runner.build", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("spotlight_runner_trust.cluster", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("spotlight_runner.issued", plancheck.ResourceActionNoop),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_runner.build", "labels.#", "2"),
					resource.TestCheckResourceAttr("spotlight_runner_trust.cluster", "labels.#", "2"),
					acctest.NothingHolds(acctest.Value(first), acctest.Value(second)),
				),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + runnerAccConfig(k, `"tf-acc-vpc", "tf-acc-gpu"`, 2),
				ConfigVariables:  runnerAccVariables(second),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:            "spotlight_runner.build",
				ImportState:             true,
				ImportStateId:           acctest.ImportID(id, k.Runner),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"token_version"},
				ConfigVariables:         runnerAccVariables(second),
			},
			{
				ResourceName:      "spotlight_runner.issued",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.IssuedRunner),
				ImportStateVerify: true,
				ConfigVariables:   runnerAccVariables(second),
			},
			{
				ResourceName:      "spotlight_runner_trust.cluster",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Trust),
				ImportStateVerify: true,
				ConfigVariables:   runnerAccVariables(second),
			},
			{
				Config:          acctest.ReleasedWorkspace(cp.Workspace) + runnerAccConfig(k, `"tf-acc-vpc", "tf-acc-gpu"`, 2),
				ConfigVariables: runnerAccVariables(second),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					acctest.ExpectForgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction("spotlight_runner.build", plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}

func TestRunnerImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            acctest.Workspace + runnerMinted,
				ConfigStateChecks: acctest.ExpectIdentity(runnerAddress),
			},
			acctest.ImportedByIdentity(runnerAddress),
		},
	})
}
