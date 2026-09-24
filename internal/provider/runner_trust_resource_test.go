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

const runnerTrustAddress = "spotlight_runner_trust.cluster"

func runnerTrustConfig(key, subject, labels string) string {
	return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_runner_trust" "cluster" {
  workspace_id = spotlight_workspace.acme.id
  key          = %q
  issuer       = "https://oidc.eks.eu-west-1.amazonaws.com/id/EXAMPLE"
  subject      = %q
  labels       = [%s]
}
`, key, subject, labels)
}

func runnerTrustImportID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/production-cluster", nil
}

func TestRunnerTrustLifecycle(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "runner-trust", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: runnerTrustConfig("production-cluster", "system:serviceaccount:spotlight:runner", `"gpu"`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerTrustAddress, tfjsonpath.New("address"), knownvalue.StringExact("production-cluster")),
					statecheck.ExpectKnownValue(runnerTrustAddress, tfjsonpath.New("audience"), knownvalue.StringExact("spotlight")),
					statecheck.ExpectKnownValue(runnerTrustAddress, tfjsonpath.New("labels"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("gpu")})),
				},
			},
			{
				Config:   runnerTrustConfig("production-cluster", "system:serviceaccount:spotlight:runner", `"gpu"`),
				PlanOnly: true,
			},
			{
				Config: runnerTrustConfig("production-cluster", "system:serviceaccount:spotlight:worker", `"gpu", "vpc-internal"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerTrustAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerTrustAddress, tfjsonpath.New("version"), knownvalue.Int64Exact(2)),
				},
			},
			{
				ResourceName:      runnerTrustAddress,
				ImportState:       true,
				ImportStateIdFunc: runnerTrustImportID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("runner-trust", "production-cluster"), func(fields map[string]any) { fields["audience"] = "sts.amazonaws.com" })
				},
				Config: runnerTrustConfig("production-cluster", "system:serviceaccount:spotlight:worker", `"gpu", "vpc-internal"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerTrustAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(runnerTrustAddress, tfjsonpath.New("audience"), knownvalue.StringExact("spotlight")),
				},
			},
			{
				Config: runnerTrustConfig("staging-cluster", "system:serviceaccount:spotlight:worker", `"gpu"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(runnerTrustAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
			},
		},
	})
}

func TestRunnerTrustWritesNeedTheRunnersScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.Grant("configure") },
				Config:      runnerTrustConfig("production-cluster", "system:serviceaccount:spotlight:runner", ""),
				ExpectError: regexp.MustCompile(`(?s)lacks the runners scope`),
			},
		},
	})
}

func TestRunnerTrustImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            runnerTrustConfig("production-cluster", "system:serviceaccount:spotlight:runner", `"gpu"`),
				ConfigStateChecks: acctest.ExpectIdentity(runnerTrustAddress),
			},
			acctest.ImportedByIdentity(runnerTrustAddress),
		},
	})
}
