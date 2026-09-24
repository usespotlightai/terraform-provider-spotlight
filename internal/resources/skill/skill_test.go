package skill_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/providertest"
)

const address = "spotlight_skill.grafana"

func config(description string) string {
	return providertest.Workspace + fmt.Sprintf(`
resource "spotlight_skill" "grafana" {
  team_id = spotlight_team.core.id
  key     = "grafana"
  files = {
    "SKILL.md"               = "---\nname: grafana\ndescription: %s\n---\nSearch before you guess.\n"
    "queries/latency.promql" = "histogram_quantile(0.99, rate(x[5m]))"
  }
}
`, description)
}

func importID(state *terraform.State) (string, error) {
	workspace := state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID

	return workspace + "/core/grafana", nil
}

func TestSkillLifecycle(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		CheckDestroy:             providertest.NoneLeft(fake, "skill", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: config("Look up dashboards"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "address", "core/grafana"),
					resource.TestCheckResourceAttr(address, "files.%", "2"),
					resource.TestCheckResourceAttr(address, "description", "Look up dashboards"),
					resource.TestCheckResourceAttr(address, "tools.#", "0"),
					resource.TestCheckResourceAttr(address, "secrets.#", "0"),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateIdFunc: importID,
				ImportStateVerify: true,
			},
			{
				Config: config("Find the right dashboard"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(address, tfjsonpath.New("description")),
					},
				},
				Check: resource.TestCheckResourceAttr(address, "description", "Find the right dashboard"),
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("skill", "core/grafana"), func(fields map[string]any) {
						fields["files"] = map[string]any{"SKILL.md": "---\nname: grafana\ndescription: changed\n---\n"}
					})
				},
				Config: config("Find the right dashboard"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(address, "files.%", "2"),
			},
		},
	})
}
