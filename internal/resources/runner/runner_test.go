package runner_test

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
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

const address = "spotlight_runner.build"

var (
	writeOnly = []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)}
	first     = strings.Repeat("a", 40)
	second    = strings.Repeat("b", 40)
)

func withToken(name, labels, token string, version int) string {
	return providertest.Workspace + fmt.Sprintf(`
resource "spotlight_runner" "build" {
  workspace_id  = spotlight_workspace.acme.id
  name          = %q
  labels        = [%s]
  token         = %q
  token_version = %d
}
`, name, labels, token, version)
}

func importID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/build-1", nil
}

func keeps(fake *fakeapi.Server, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := fake.Kept(fake.IDOf("runner", "build-1")); got != want {
			return fmt.Errorf("the runner calls with %q, want %q", got, want)
		}

		return nil
	}
}

func tokensSent(fake *fakeapi.Server) int {
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
		TerraformVersionChecks:   writeOnly,
		ProtoV6ProviderFactories: providertest.Factories(fake),
		CheckDestroy:             providertest.NoneLeft(fake, "runner", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: withToken("build-1", `"vpc-internal"`, first, 1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("address"), knownvalue.StringExact("build-1")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("token"), knownvalue.Null()),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("trust"), knownvalue.Null()),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("labels"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("vpc-internal")})),
				},
				Check: resource.ComposeAggregateTestCheckFunc(keeps(fake, first), providertest.NothingHolds(providertest.Value(first))),
			},
			{
				Config:   withToken("build-1", `"vpc-internal"`, first, 1),
				PlanOnly: true,
			},
			{
				Config: withToken("build-1", `"vpc-internal", "gpu"`, first, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				Check: func(*terraform.State) error {
					if sent := tokensSent(fake); sent != 1 {
						return fmt.Errorf("the token was sent %d times, want once: relabelling sends none", sent)
					}

					return nil
				},
			},
			{
				Config: withToken("build-1", `"gpu", "vpc-internal"`, second, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(keeps(fake, second), providertest.NothingHolds(providertest.Value(first), providertest.Value(second))),
			},
			{
				ResourceName:            address,
				ImportState:             true,
				ImportStateIdFunc:       importID,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"token_version"},
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("runner", "build-1"), func(fields map[string]any) { fields["labels"] = []any{} })
				},
				Config: withToken("build-1", `"gpu", "vpc-internal"`, second, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("labels"), knownvalue.SetSizeExact(2)),
				},
			},
			{
				Config: withToken("build-2", `"gpu"`, second, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroyBeforeCreate)},
				},
			},
		},
	})
}

const minted = `
resource "spotlight_runner" "build" {
  workspace_id = spotlight_workspace.acme.id
  name         = "build-1"
}
`

func TestARunnerTokenSpotlightIssuesIsNeverKept(t *testing.T) {
	fake := fakeapi.New(t)
	issued := func() string { return fake.Kept(fake.IDOf("runner", "build-1")) }

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   writeOnly,
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config: providertest.Workspace + minted,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("token"), knownvalue.Null()),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("labels"), knownvalue.SetSizeExact(0)),
				},
				Check: func(state *terraform.State) error {
					if !strings.HasPrefix(issued(), "issued-") {
						return fmt.Errorf("Spotlight issued no token")
					}

					return providertest.NothingHolds(issued)(state)
				},
			},
			{
				Config:   providertest.Workspace + minted,
				PlanOnly: true,
			},
			{
				Config: withToken("build-1", "", first, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				Check: keeps(fake, first),
			},
		},
	})
}

func TestRunnerWritesNeedTheRunnersScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   writeOnly,
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.Grant("configure", "secrets") },
				Config:      withToken("build-1", "", first, 1),
				ExpectError: regexp.MustCompile(`(?s)Not allowed to change the runner.*lacks the runners scope`),
			},
		},
	})
}

func TestARunnerTokenMustBeLongEnough(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   writeOnly,
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config:      withToken("build-1", "", "short", 1),
				ExpectError: regexp.MustCompile(`(?s)token: is 32 to 255 URL-safe characters`),
			},
		},
	})
}
