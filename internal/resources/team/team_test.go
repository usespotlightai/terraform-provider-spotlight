package team_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/providertest"
)

const address = "spotlight_team.core"

func config(key, name string) string {
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

func importID(state *terraform.State) (string, error) {
	team := state.RootModule().Resources[address].Primary.Attributes

	return team["workspace_id"] + "/" + team["key"], nil
}

func TestTeamLifecycle(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		CheckDestroy:             providertest.NoneLeft(fake, "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: config("core", "Core"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(address, "workspace_id", "spotlight_workspace.acme", "id"),
					resource.TestCheckResourceAttr(address, "key", "core"),
					resource.TestCheckResourceAttr(address, "name", "Core"),
					resource.TestCheckResourceAttr(address, "address", "core"),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateIdFunc: importID,
				ImportStateVerify: true,
			},
			{
				Config: config("core", "Core Platform"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(address, "version", "2"),
			},
			{
				Config: config("platform", "Core Platform"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.TestCheckResourceAttr(address, "address", "platform"),
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("team", "platform"), func(fields map[string]any) { fields["name"] = "Someone else's name" })
				},
				Config: config("platform", "Core Platform"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(address, "name", "Core Platform"),
			},
		},
	})
}

func TestTeamImportNeedsTheWorkspace(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config:        config("core", "Core"),
				ResourceName:  address,
				ImportState:   true,
				ImportStateId: "core",
				ExpectError:   regexp.MustCompile(`Import this resource by its workspace id and address`),
			},
		},
	})
}
