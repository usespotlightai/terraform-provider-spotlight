package acctest_test

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
)

func factories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"spotlight": providerserver.NewProtocol6WithError(provider.New("acc")()),
	}
}

func needsControlPlane(t *testing.T) {
	t.Helper()

	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skipf("acceptance tests run only with %s=1", resource.EnvTfAcc)
	}

	for _, env := range []string{provider.EndpointEnv, provider.TokenEnv} {
		if os.Getenv(env) == "" {
			t.Skipf("acceptance tests need %s and %s for a dev control plane", provider.EndpointEnv, provider.TokenEnv)
		}
	}
}

func workspaceConfig(slug string) string {
	return fmt.Sprintf(`
resource "spotlight_workspace" "acc" {
  name = "Terraform acceptance %[1]s"
  slug = %[1]q
}

resource "spotlight_team" "core" {
  workspace_id = spotlight_workspace.acc.id
  key          = "core"
  name         = "Core"
}

resource "spotlight_skill" "grafana" {
  team_id = spotlight_team.core.id
  key     = "grafana"
  files = {
    "SKILL.md" = <<-EOT
      ---
      name: grafana
      description: Look up dashboards
      ---
      Search before you guess.
    EOT
  }
}

resource "spotlight_integration" "linear" {
  team_id = spotlight_team.core.id
  definition = {
    integration = "linear"
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
        does        = "comment"
        description = "Comment on the issue."
        inputs      = { body = { type = "string", required = true } }
      }
    }
  }
}

resource "spotlight_line" "delivery" {
  team_id      = spotlight_team.core.id
  key          = "delivery"
  name         = "Delivery"
  repositories = ["acme/api"]

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
    prompt    = "Implement the task.\n"
    check     = "npm test"
    skills    = [spotlight_skill.grafana.key]
    passes_to = ["completed"]
  }

  station {
    key  = "completed"
    name = "Completed"
    kind = "completed"
    ends = true
  }

  reaction {
    happening = "github.checks.failed"
    from      = "implementation"
    to        = "implementation"
  }
}
`, slug)
}

func importID(address string) resource.ImportStateIdFunc {
	return func(state *terraform.State) (string, error) {
		workspace := state.RootModule().Resources["spotlight_workspace.acc"].Primary.ID

		return workspace + "/" + address, nil
	}
}

func TestAccWorkspaceConfiguredEndToEnd(t *testing.T) {
	needsControlPlane(t)

	slug := "tf-acc-" + strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlpha))
	config := workspaceConfig(slug)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_workspace.acc", "address", slug),
					resource.TestCheckResourceAttr("spotlight_team.core", "address", "core"),
					resource.TestCheckResourceAttr("spotlight_line.delivery", "address", "core/delivery"),
					resource.TestCheckResourceAttr("spotlight_skill.grafana", "description", "Look up dashboards"),
					resource.TestCheckResourceAttr("spotlight_integration.linear", "source", "linear"),
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "spotlight_workspace.acc",
				ImportState:       true,
				ImportStateId:     slug,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "spotlight_team.core",
				ImportState:       true,
				ImportStateIdFunc: importID("core"),
				ImportStateVerify: true,
			},
			{
				ResourceName:      "spotlight_line.delivery",
				ImportState:       true,
				ImportStateIdFunc: importID("core/delivery"),
				ImportStateVerify: true,
			},
			{
				ResourceName:      "spotlight_skill.grafana",
				ImportState:       true,
				ImportStateIdFunc: importID("core/grafana"),
				ImportStateVerify: true,
			},
			{
				ResourceName:            "spotlight_integration.linear",
				ImportState:             true,
				ImportStateIdFunc:       importID("core/linear"),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"definition"},
			},
		},
	})
}

func TestAccBadExpressionFailsPlanAtItsAttribute(t *testing.T) {
	needsControlPlane(t)

	slug := "tf-acc-" + strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlpha))
	config := strings.Replace(workspaceConfig(slug), "$${{ inputs.body }}", "$${{ inputs.bdoy }}", 1)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      config,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid expression in\s+definition\.actions\.comment\.request\.body\.query`),
			},
		},
	})
}
