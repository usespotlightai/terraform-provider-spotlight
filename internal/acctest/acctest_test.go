package acctest_test

import (
	"context"
	"errors"
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

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const workspaceEnv = "SPOTLIGHT_ACC_WORKSPACE"

func factories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"spotlight": providerserver.NewProtocol6WithError(provider.New("acc")()),
	}
}

type controlPlane struct {
	api       spotlight.API
	workspace spotlight.Workspace
}

func needsControlPlane(t *testing.T) controlPlane {
	t.Helper()

	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skipf("acceptance tests run only with %s=1", resource.EnvTfAcc)
	}

	for _, env := range []string{provider.EndpointEnv, provider.TokenEnv, workspaceEnv} {
		if os.Getenv(env) == "" {
			t.Skipf("acceptance tests need %s, %s and %s: a dev control plane, a token there and the slug of its workspace", provider.EndpointEnv, provider.TokenEnv, workspaceEnv)
		}
	}

	api, err := client.New(client.Config{
		Endpoint:        os.Getenv(provider.EndpointEnv),
		Token:           os.Getenv(provider.TokenEnv),
		ProviderVersion: "acc",
		Retries:         2,
		Backoff:         client.DefaultBackoff,
	})
	if err != nil {
		t.Fatal(err)
	}

	workspace, err := api.Workspaces.Import(t.Context(), "", os.Getenv(workspaceEnv))
	if err != nil {
		t.Fatalf("the token cannot see the workspace %s: %v", os.Getenv(workspaceEnv), err)
	}

	return controlPlane{api: api, workspace: workspace}
}

type keys struct {
	team, line, skill, integration, profile string
	secret, awsSecret                       string
	runner, issuedRunner, trust             string
}

func randomKeys() keys {
	suffix := strings.ToLower(acctest.RandStringFromCharSet(10, acctest.CharSetAlpha))
	upper := strings.ToUpper(suffix)

	return keys{
		team:         "tf-acc-" + suffix,
		line:         "delivery-" + suffix,
		skill:        "grafana-" + suffix,
		integration:  "tfacc" + suffix,
		profile:      "careful-" + suffix,
		secret:       "TF_ACC_" + upper,
		awsSecret:    "TF_ACC_AWS_" + upper,
		runner:       "tf-acc-build-" + suffix,
		issuedRunner: "tf-acc-issued-" + suffix,
		trust:        "tf-acc-cluster-" + suffix,
	}
}

func adopted(workspace spotlight.Workspace) string {
	return fmt.Sprintf(`
import {
  to = spotlight_workspace.acc
  id = %[2]q
}

resource "spotlight_workspace" "acc" {
  name = %[1]q
  slug = %[2]q

  lifecycle {
    prevent_destroy = true
  }
}

locals {
  workspace_id = spotlight_workspace.acc.id
}
`, workspace.Name, workspace.Slug)
}

func released(workspace spotlight.Workspace) string {
	return fmt.Sprintf(`
removed {
  from = spotlight_workspace.acc

  lifecycle {
    destroy = false
  }
}

locals {
  workspace_id = %q
}
`, workspace.ID)
}

func teamConfig(k keys) string {
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
        does        = "comment"
        description = "Comment on the issue."
        inputs      = { body = { type = "string", required = true } }
      }
    }
  }
}

resource "spotlight_line" "delivery" {
  team_id      = spotlight_team.acc.id
  key          = %[4]q
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
`, k.team, k.skill, k.integration, k.line)
}

func importID(workspaceID, address string) string {
	return workspaceID + "/" + address
}

type forgotten string

func (address forgotten) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	for _, each := range req.Plan.ResourceChanges {
		if each.Address == string(address) {
			if !each.Change.Actions.Forget() {
				resp.Error = fmt.Errorf("%s is planned to %v, not to be forgotten", address, each.Change.Actions)
			}

			return
		}
	}

	resp.Error = fmt.Errorf("%s is not in the plan", address)
}

type held struct {
	kind, address string
	find          func(context.Context) (spotlight.Meta, error)
	remove        func(context.Context, spotlight.Meta) error
}

func holding[In, Out any](kind string, resources spotlight.Resources[In, Out], metaOf func(Out) spotlight.Meta, workspaceID, address string) held {
	return held{
		kind:    kind,
		address: address,
		find: func(ctx context.Context) (spotlight.Meta, error) {
			found, err := resources.Import(ctx, workspaceID, address)

			return metaOf(found), err
		},
		remove: func(ctx context.Context, meta spotlight.Meta) error {
			return resources.Delete(ctx, meta.ID, meta.Version)
		},
	}
}

func (cp controlPlane) created(k keys) []held {
	id := cp.workspace.ID

	return []held{
		holding("line", cp.api.Lines, func(l spotlight.Line) spotlight.Meta { return l.Meta }, id, k.team+"/"+k.line),
		holding("skill", cp.api.Skills, func(s spotlight.Skill) spotlight.Meta { return s.Meta }, id, k.team+"/"+k.skill),
		holding("integration", cp.api.Integrations, func(i spotlight.Integration) spotlight.Meta { return i.Meta }, id, k.team+"/"+k.integration),
		holding("harness profile", cp.api.HarnessProfiles, func(p spotlight.HarnessProfile) spotlight.Meta { return p.Meta }, id, k.team+"/"+k.profile),
		holding("team", cp.api.Teams, func(t spotlight.Team) spotlight.Meta { return t.Meta }, id, k.team),
		holding("secret", cp.api.Secrets, func(s spotlight.Secret) spotlight.Meta { return s.Meta }, id, k.secret),
		holding("secret", cp.api.Secrets, func(s spotlight.Secret) spotlight.Meta { return s.Meta }, id, k.awsSecret),
		holding("runner", cp.api.Runners, func(r spotlight.Runner) spotlight.Meta { return r.Meta }, id, k.runner),
		holding("runner", cp.api.Runners, func(r spotlight.Runner) spotlight.Meta { return r.Meta }, id, k.issuedRunner),
		holding("runner trust", cp.api.RunnerTrusts, func(t spotlight.RunnerTrust) spotlight.Meta { return t.Meta }, id, k.trust),
	}
}

func (cp controlPlane) sweep(t *testing.T, k keys) {
	t.Logf("working in workspace %s under team %s", cp.workspace.Slug, k.team)

	t.Cleanup(func() {
		ctx := context.Background()

		for _, each := range cp.created(k) {
			meta, err := each.find(ctx)
			if spotlight.IsNotFound(err) {
				continue
			}
			if err == nil {
				err = each.remove(ctx, meta)
			}
			if err != nil {
				t.Errorf("the %s %s was left behind: %v", each.kind, each.address, err)
			}
		}
	})
}

func (cp controlPlane) nothingLeftButTheWorkspace(k keys) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()

		workspace, err := cp.api.Workspaces.Import(ctx, "", cp.workspace.Slug)
		if err != nil {
			return fmt.Errorf("the workspace %s did not survive: %w", cp.workspace.Slug, err)
		}

		if workspace.ID != cp.workspace.ID || workspace.Version != cp.workspace.Version || workspace.Name != cp.workspace.Name {
			return fmt.Errorf("the workspace changed: %+v, was %+v", workspace, cp.workspace)
		}

		var left []error
		for _, each := range cp.created(k) {
			if _, err := each.find(ctx); !spotlight.IsNotFound(err) {
				left = append(left, fmt.Errorf("the %s %s is still there (%v)", each.kind, each.address, err))
			}
		}

		return errors.Join(left...)
	}
}

func TestAccWorkspaceConfiguredEndToEnd(t *testing.T) {
	cp := needsControlPlane(t)
	k := randomKeys()
	cp.sweep(t, k)

	managed := adopted(cp.workspace) + teamConfig(k)
	id := cp.workspace.ID

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		CheckDestroy:             cp.nothingLeftButTheWorkspace(k),
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
					resource.TestCheckResourceAttr("spotlight_workspace.acc", "address", cp.workspace.Slug),
					resource.TestCheckResourceAttr("spotlight_team.acc", "address", k.team),
					resource.TestCheckResourceAttr("spotlight_line.delivery", "address", k.team+"/"+k.line),
					resource.TestCheckResourceAttr("spotlight_skill.grafana", "description", "Look up dashboards"),
					resource.TestCheckResourceAttr("spotlight_integration.linear", "source", k.integration),
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
				ImportStateId:     cp.workspace.Slug,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "spotlight_team.acc",
				ImportState:       true,
				ImportStateId:     importID(id, k.team),
				ImportStateVerify: true,
			},
			{
				ResourceName:      "spotlight_line.delivery",
				ImportState:       true,
				ImportStateId:     importID(id, k.team+"/"+k.line),
				ImportStateVerify: true,
			},
			{
				ResourceName:      "spotlight_skill.grafana",
				ImportState:       true,
				ImportStateId:     importID(id, k.team+"/"+k.skill),
				ImportStateVerify: true,
			},
			{
				ResourceName:            "spotlight_integration.linear",
				ImportState:             true,
				ImportStateId:           importID(id, k.team+"/"+k.integration),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"definition"},
			},
			{
				Config: released(cp.workspace) + teamConfig(k),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						forgotten("spotlight_workspace.acc"),
						plancheck.ExpectResourceAction("spotlight_team.acc", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("spotlight_line.delivery", plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

func TestAccBadExpressionFailsPlanAtItsAttribute(t *testing.T) {
	cp := needsControlPlane(t)
	k := randomKeys()
	cp.sweep(t, k)

	config := strings.Replace(adopted(cp.workspace)+teamConfig(k), "$${{ inputs.body }}", "$${{ inputs.bdoy }}", 1)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		CheckDestroy:             cp.nothingLeftButTheWorkspace(k),
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
	needsControlPlane(t)

	slug := "tf-acc-" + strings.ToLower(acctest.RandStringFromCharSet(10, acctest.CharSetAlpha))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "spotlight_workspace" "acc" {
  name = "Terraform acceptance %[1]s"
  slug = %[1]q
}
`, slug),
				ExpectError: regexp.MustCompile(`may not open a new\s+one(?s:.*)import it\s+rather than creating it`),
			},
		},
	})
}
