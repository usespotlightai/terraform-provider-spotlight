package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

const skillAddress = "spotlight_skill.grafana"

func skillConfig(description string) string {
	return acctest.Workspace + fmt.Sprintf(`
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

func skillImportID(state *terraform.State) (string, error) {
	workspace := state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID

	return workspace + "/core/grafana", nil
}

func TestSkillLifecycle(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "skill", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: skillConfig("Look up dashboards"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(skillAddress, "address", "core/grafana"),
					resource.TestCheckResourceAttr(skillAddress, "files.%", "2"),
					resource.TestCheckResourceAttr(skillAddress, "description", "Look up dashboards"),
					resource.TestCheckResourceAttr(skillAddress, "tools.#", "0"),
					resource.TestCheckResourceAttr(skillAddress, "secrets.#", "0"),
				),
			},
			{
				ResourceName:      skillAddress,
				ImportState:       true,
				ImportStateIdFunc: skillImportID,
				ImportStateVerify: true,
			},
			{
				Config: skillConfig("Find the right dashboard"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(skillAddress, plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(skillAddress, tfjsonpath.New("description")),
					},
				},
				Check: resource.TestCheckResourceAttr(skillAddress, "description", "Find the right dashboard"),
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("skill", "core/grafana"), func(fields map[string]any) {
						fields["files"] = map[string]any{"SKILL.md": "---\nname: grafana\ndescription: changed\n---\n"}
					})
				},
				Config: skillConfig("Find the right dashboard"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(skillAddress, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(skillAddress, "files.%", "2"),
			},
		},
	})
}

func TestSkillImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            skillConfig("Look up dashboards"),
				ConfigStateChecks: acctest.ExpectIdentity(skillAddress),
			},
			acctest.ImportedByIdentity(skillAddress),
		},
	})
}
