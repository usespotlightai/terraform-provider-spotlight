package provider_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

const teamAddress = "spotlight_team.core"

func teamConfig(key, name string) string {
	return fmt.Sprintf(`
resource "spotlight_workspace" "acme" {
  name = "Acme"
  slug = "acme"
}

resource "spotlight_team" "core" {
  workspace_id = spotlight_workspace.acme.id
  key          = %q
  name         = %q
}
`, key, name)
}

func teamImportID(state *terraform.State) (string, error) {
	team := state.RootModule().Resources[teamAddress].Primary.Attributes

	return team["workspace_id"] + "/" + team["key"], nil
}

func TestTeamLifecycle(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: teamConfig("core", "Core"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(teamAddress, "workspace_id", "spotlight_workspace.acme", "id"),
					resource.TestCheckResourceAttr(teamAddress, "key", "core"),
					resource.TestCheckResourceAttr(teamAddress, "name", "Core"),
					resource.TestCheckResourceAttr(teamAddress, "address", "core"),
				),
			},
			{
				ResourceName:      teamAddress,
				ImportState:       true,
				ImportStateIdFunc: teamImportID,
				ImportStateVerify: true,
			},
			{
				Config: teamConfig("core", "Core Platform"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(teamAddress, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(teamAddress, "version", "2"),
			},
			{
				Config: teamConfig("platform", "Core Platform"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(teamAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.TestCheckResourceAttr(teamAddress, "address", "platform"),
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("team", "platform"), func(fields map[string]any) { fields["name"] = "Someone else's name" })
				},
				Config: teamConfig("platform", "Core Platform"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(teamAddress, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(teamAddress, "name", "Core Platform"),
			},
		},
	})
}

func TestTeamImportNeedsTheWorkspace(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:        teamConfig("core", "Core"),
				ResourceName:  teamAddress,
				ImportState:   true,
				ImportStateId: "core",
				ExpectError:   regexp.MustCompile(`Import this resource by its workspace id and address`),
			},
		},
	})
}

func TestTeamImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            teamConfig("core", "Core"),
				ConfigStateChecks: acctest.ExpectIdentity(teamAddress),
			},
			acctest.ImportedByIdentity(teamAddress),
		},
	})
}
