package provider_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
)

func endToEndConfig(k acctest.Keys) string {
	return fmt.Sprintf(`
resource "spotlight_team" "acc" {
  workspace_id = local.workspace_id
  key          = %[1]q
  name         = "Terraform acceptance %[1]s"
}

resource "spotlight_skill" "grafana" {
  team_id = spotlight_team.acc.id
  key     = %[2]q
  files = {
    "SKILL.md" = <<-EOT
      ---
      name: %[2]s
      description: Look up dashboards
      ---
      Search before you guess.
    EOT
  }
}

resource "spotlight_integration" "linear" {
  team_id = spotlight_team.acc.id
  definition = {
    integration = %[3]q
    actions = {
      comment = {
        request = {
          method  = "POST"
          url     = "https://api.linear.app/graphql"
          headers = { "Content-Type" = "application/json" }
          body    = { query = "$${{ inputs.body }}" }
        }
      }
    }
    tools = {
      comment = {
        action      = "comment"
        description = "Comment on the issue."
        inputs      = { body = { type = "string", required = true } }
      }
    }
  }
}

resource "spotlight_line" "delivery" {
  team_id = spotlight_team.acc.id
  key     = %[4]q
  name    = "Delivery"

  repository {
    name  = "acme/api"
    setup = "npm ci"
  }

  station {
    key       = "backlog"
    name      = "Backlog"
    kind      = "backlog"
    passes_to = ["implementation"]
  }

  station {
    key       = "implementation"
    name      = "Implementation"
    harness   = "claude-code"
    prompt    = "Implement the task."
    check     = "npm test"
    skills    = [spotlight_skill.grafana.key]
    passes_to = ["code-review"]
  }

  station {
    key       = "code-review"
    name      = "Code review"
    harness   = "claude-code"
    prompt    = "Review the change."
    passes_to = ["implementation"]
    ends      = true
  }

  station {
    key  = "completed"
    name = "Completed"
    kind = "completed"
  }

  reaction {
    happening = "github.checks.failed"
    from      = "implementation"
    to        = "implementation"
  }
}
`, k.Team, k.Skill, k.Integration, k.Line)
}

func TestAccWorkspaceConfiguredEndToEnd(t *testing.T) {
	cp := acctest.PreCheck(t)
	k := acctest.RandomKeys()
	cp.Sweep(t, k)

	managed := acctest.AdoptedWorkspace(cp.Workspace) + endToEndConfig(k)
	id := cp.Workspace.ID

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config: managed,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("spotlight_workspace.acc", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("spotlight_team.acc", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_workspace.acc", "id", id),
					resource.TestCheckResourceAttr("spotlight_workspace.acc", "address", cp.Workspace.Slug),
					resource.TestCheckResourceAttr("spotlight_team.acc", "address", k.Team),
					resource.TestCheckResourceAttr("spotlight_line.delivery", "address", k.Team+"/"+k.Line),
					resource.TestCheckResourceAttr("spotlight_skill.grafana", "description", "Look up dashboards"),
					resource.TestCheckResourceAttr("spotlight_integration.linear", "integration", k.Integration),
				),
			},
			{
				Config: managed,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "spotlight_workspace.acc",
				ImportState:       true,
				ImportStateId:     cp.Workspace.Slug,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "spotlight_team.acc",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Team),
				ImportStateVerify: true,
			},
			{
				ResourceName:      "spotlight_line.delivery",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Team+"/"+k.Line),
				ImportStateVerify: true,
			},
			{
				ResourceName:      "spotlight_skill.grafana",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Team+"/"+k.Skill),
				ImportStateVerify: true,
			},
			{
				ResourceName:            "spotlight_integration.linear",
				ImportState:             true,
				ImportStateId:           acctest.ImportID(id, k.Team+"/"+k.Integration),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"definition"},
			},
			{
				Config: acctest.ReleasedWorkspace(cp.Workspace) + endToEndConfig(k),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						acctest.ExpectForgotten("spotlight_workspace.acc"),
						plancheck.ExpectResourceAction("spotlight_team.acc", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("spotlight_line.delivery", plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

func TestAccBadExpressionFailsPlanAtItsAttribute(t *testing.T) {
	cp := acctest.PreCheck(t)
	k := acctest.RandomKeys()
	cp.Sweep(t, k)

	config := strings.Replace(acctest.AdoptedWorkspace(cp.Workspace)+endToEndConfig(k), "$${{ inputs.body }}", "$${{ inputs.bdoy }}", 1)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config:      config,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid expression in\s+definition\.actions\.comment\.request\.body\.query`),
			},
		},
	})
}

func TestAccTokenMayNotCreateAWorkspace(t *testing.T) {
	acctest.PreCheck(t)

	slug := acctest.RandomKeys().Team

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "spotlight_workspace" "acc" {
  name = "Terraform acceptance %[1]s"
  slug = %[1]q
}
`, slug),
				ExpectError: regexp.MustCompile(`may not create a new\s+one(?s:.*)import it\s+rather than creating it`),
			},
		},
	})
}
