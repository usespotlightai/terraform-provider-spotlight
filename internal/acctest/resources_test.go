package acctest_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/providertest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

var writeOnly = []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)}

func randomToken() string {
	return "tfacc" + acctest.RandStringFromCharSet(40, acctest.CharSetAlphaNum)
}

func secretConfig(k keys, bindings string, version int) string {
	return fmt.Sprintf(`
variable "secret_value" {
  type      = string
  sensitive = true
}

resource "spotlight_secret" "kept" {
  workspace_id     = local.workspace_id
  name             = %[1]q
  bindings         = [%[3]s]
  value_wo         = var.secret_value
  value_wo_version = %[4]d
}

resource "spotlight_secret" "aws" {
  workspace_id = local.workspace_id
  name         = %[2]q
  source = {
    kind      = "aws-secrets-manager"
    secret_id = "tf-acc/%[2]s"
    field     = "token"
  }
}
`, k.secret, k.awsSecret, bindings, version)
}

func valued(value string) config.Variables {
	return config.Variables{"secret_value": config.StringVariable(value)}
}

func TestAccSecret(t *testing.T) {
	cp := needsControlPlane(t)
	k := randomKeys()
	cp.sweep(t, k)

	first, second := randomToken(), randomToken()
	id := cp.workspace.ID
	linear := `{ host = "api.linear.app", prefix = "Bearer " }`
	both := linear + `, { host = "api.github.com", header = "x-api-key" }`

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   writeOnly,
		ProtoV6ProviderFactories: factories(),
		CheckDestroy:             cp.nothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config:          adopted(cp.workspace) + secretConfig(k, linear, 1),
				ConfigVariables: valued(first),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_secret.kept", "address", k.secret),
					resource.TestCheckResourceAttr("spotlight_secret.kept", "source.kind", "built-in"),
					resource.TestCheckResourceAttr("spotlight_secret.kept", "bindings.0.header", "authorization"),
					resource.TestCheckResourceAttr("spotlight_secret.kept", "value_wo_version", "1"),
					resource.TestCheckNoResourceAttr("spotlight_secret.kept", "value_wo"),
					resource.TestCheckResourceAttr("spotlight_secret.aws", "source.secret_id", "tf-acc/"+k.awsSecret),
					providertest.NothingHolds(providertest.Value(first)),
				),
			},
			{
				Config:           adopted(cp.workspace) + secretConfig(k, linear, 1),
				ConfigVariables:  valued(first),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config:          adopted(cp.workspace) + secretConfig(k, both, 2),
				ConfigVariables: valued(second),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("spotlight_secret.kept", plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_secret.kept", "bindings.#", "2"),
					resource.TestCheckResourceAttr("spotlight_secret.kept", "value_wo_version", "2"),
					providertest.NothingHolds(providertest.Value(first), providertest.Value(second)),
				),
			},
			{
				Config:           adopted(cp.workspace) + secretConfig(k, both, 2),
				ConfigVariables:  valued(second),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      "spotlight_secret.kept",
				ImportState:       true,
				ImportStateId:     importID(id, k.secret),
				ImportStateVerify: true,
				ConfigVariables:   valued(second),
			},
			{
				ResourceName:      "spotlight_secret.aws",
				ImportState:       true,
				ImportStateId:     importID(id, k.awsSecret),
				ImportStateVerify: true,
				ConfigVariables:   valued(second),
			},
			{
				Config:          released(cp.workspace) + secretConfig(k, both, 2),
				ConfigVariables: valued(second),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					forgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction("spotlight_secret.kept", plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}

func tokened(token string) config.Variables {
	return config.Variables{"runner_token": config.StringVariable(token)}
}

func runnerConfig(k keys, labels string, version int) string {
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
`, k.runner, k.issuedRunner, labels, version, k.trust)
}

func TestAccRunnerAndRunnerTrust(t *testing.T) {
	cp := needsControlPlane(t)
	k := randomKeys()
	cp.sweep(t, k)

	first, second := randomToken(), randomToken()
	id := cp.workspace.ID

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   writeOnly,
		ProtoV6ProviderFactories: factories(),
		CheckDestroy:             cp.nothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config:          adopted(cp.workspace) + runnerConfig(k, `"tf-acc-gpu"`, 1),
				ConfigVariables: tokened(first),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_runner.build", "address", k.runner),
					resource.TestCheckResourceAttr("spotlight_runner.build", "labels.#", "1"),
					resource.TestCheckNoResourceAttr("spotlight_runner.build", "token"),
					resource.TestCheckNoResourceAttr("spotlight_runner.build", "trust"),
					resource.TestCheckNoResourceAttr("spotlight_runner.issued", "token"),
					resource.TestCheckResourceAttr("spotlight_runner_trust.cluster", "audience", "spotlight"),
					providertest.NothingHolds(providertest.Value(first)),
				),
			},
			{
				Config:           adopted(cp.workspace) + runnerConfig(k, `"tf-acc-gpu"`, 1),
				ConfigVariables:  tokened(first),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config:          adopted(cp.workspace) + runnerConfig(k, `"tf-acc-gpu", "tf-acc-vpc"`, 2),
				ConfigVariables: tokened(second),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("spotlight_runner.build", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("spotlight_runner_trust.cluster", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("spotlight_runner.issued", plancheck.ResourceActionNoop),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_runner.build", "labels.#", "2"),
					resource.TestCheckResourceAttr("spotlight_runner_trust.cluster", "labels.#", "2"),
					providertest.NothingHolds(providertest.Value(first), providertest.Value(second)),
				),
			},
			{
				Config:           adopted(cp.workspace) + runnerConfig(k, `"tf-acc-vpc", "tf-acc-gpu"`, 2),
				ConfigVariables:  tokened(second),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:            "spotlight_runner.build",
				ImportState:             true,
				ImportStateId:           importID(id, k.runner),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"token_version"},
				ConfigVariables:         tokened(second),
			},
			{
				ResourceName:      "spotlight_runner.issued",
				ImportState:       true,
				ImportStateId:     importID(id, k.issuedRunner),
				ImportStateVerify: true,
				ConfigVariables:   tokened(second),
			},
			{
				ResourceName:      "spotlight_runner_trust.cluster",
				ImportState:       true,
				ImportStateId:     importID(id, k.trust),
				ImportStateVerify: true,
				ConfigVariables:   tokened(second),
			},
			{
				Config:          released(cp.workspace) + runnerConfig(k, `"tf-acc-vpc", "tf-acc-gpu"`, 2),
				ConfigVariables: tokened(second),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					forgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction("spotlight_runner.build", plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}

func declarationConfig(k keys, withTrust bool) string {
	trusts := ""
	if withTrust {
		trusts = "spotlight_runner_trust.cluster.key"
	}

	return runnerConfig(k, `"tf-acc-gpu"`, 1) + fmt.Sprintf(`
resource "spotlight_workspace_runners" "acc" {
  workspace_id = local.workspace_id
  runners      = [spotlight_runner.build.name]
  trusts       = [%s]
}
`, trusts)
}

func (cp controlPlane) declared(ctx context.Context) (spotlight.WorkspaceRunners, error) {
	return cp.api.WorkspaceRunners.Import(ctx, cp.workspace.ID, cp.workspace.Slug)
}

func TestAccWorkspaceRunners(t *testing.T) {
	cp := needsControlPlane(t)

	if _, err := cp.declared(t.Context()); !spotlight.IsNotFound(err) {
		t.Skipf("the workspace %s already declares its runners (%v); this test declares them only where nobody has", cp.workspace.Slug, err)
	}

	k := randomKeys()
	token := randomToken()
	cp.sweep(t, k)
	t.Cleanup(func() {
		ctx := context.Background()
		if held, err := cp.declared(ctx); err == nil {
			if err := cp.api.WorkspaceRunners.Delete(ctx, held.ID, held.Version); err != nil {
				t.Errorf("the declaration of %s's runners was left behind: %v", cp.workspace.Slug, err)
			}
		}
	})

	nothingDeclared := func(state *terraform.State) error {
		if _, err := cp.declared(context.Background()); !spotlight.IsNotFound(err) {
			return fmt.Errorf("the declaration of the workspace's runners is still there (%v)", err)
		}

		return cp.nothingLeftButTheWorkspace(k)(state)
	}

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   writeOnly,
		ProtoV6ProviderFactories: factories(),
		CheckDestroy:             nothingDeclared,
		Steps: []resource.TestStep{
			{
				Config:          adopted(cp.workspace) + declarationConfig(k, false),
				ConfigVariables: tokened(token),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_workspace_runners.acc", "address", cp.workspace.Slug),
					resource.TestCheckResourceAttr("spotlight_workspace_runners.acc", "runners.#", "1"),
					resource.TestCheckResourceAttr("spotlight_workspace_runners.acc", "runners.0", k.runner),
				),
			},
			{
				Config:           adopted(cp.workspace) + declarationConfig(k, false),
				ConfigVariables:  tokened(token),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config:          adopted(cp.workspace) + declarationConfig(k, true),
				ConfigVariables: tokened(token),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("spotlight_workspace_runners.acc", plancheck.ResourceActionUpdate),
				}},
				Check: resource.TestCheckResourceAttr("spotlight_workspace_runners.acc", "trusts.0", k.trust),
			},
			{
				ResourceName:      "spotlight_workspace_runners.acc",
				ImportState:       true,
				ImportStateId:     importID(cp.workspace.ID, cp.workspace.Slug),
				ImportStateVerify: true,
				ConfigVariables:   tokened(token),
			},
			{
				Config:          released(cp.workspace) + declarationConfig(k, true),
				ConfigVariables: tokened(token),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					forgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction("spotlight_workspace_runners.acc", plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}

func profiledConfig(k keys, allow string, version string) string {
	return fmt.Sprintf(`
resource "spotlight_team" "acc" {
  workspace_id = local.workspace_id
  key          = %[1]q
  name         = "Terraform acceptance %[1]s"
}

resource "spotlight_harness_profile" "careful" {
  team_id = spotlight_team.acc.id
  key     = %[2]q
  permissions = {
    allow = [%[4]s]
    deny  = ["read(.env)"]
  }
  mcp_servers = [
    {
      name    = "linear"
      command = "npx"
      args    = ["-y", "linear-mcp"]
      env     = { LINEAR_WORKSPACE = "tf-acc" }
    },
  ]
  native = {
    claude-code = {
      includeCoAuthoredBy = false
    }
  }
}

resource "spotlight_line" "delivery" {
  team_id = spotlight_team.acc.id
  key     = %[3]q
  name    = "Review"
  role    = "review"
  runners = ["tf-acc-vpc"]

  station {
    key       = "backlog"
    name      = "Backlog"
    kind      = "backlog"
    passes_to = ["implementation"]
  }

  station {
    key             = "implementation"
    name            = "Implementation"
    harness         = "claude-code"
    harness_version = %[5]q
    profile         = spotlight_harness_profile.careful.key
    prompt          = "Implement the task."
    runners         = ["tf-acc-gpu"]
    passes_to       = ["code-review"]
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

  reviews {
    start = "opened"

    skip {
      authors = ["renovate[bot]", "dependabot[bot]"]
    }
  }
}
`, k.team, k.profile, k.line, allow, version)
}

func TestAccHarnessProfileAndPlacedLine(t *testing.T) {
	cp := needsControlPlane(t)
	k := randomKeys()
	cp.sweep(t, k)

	id := cp.workspace.ID

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		CheckDestroy:             cp.nothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config: adopted(cp.workspace) + profiledConfig(k, `"read"`, "2.1.270"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_harness_profile.careful", "address", k.team+"/"+k.profile),
					resource.TestCheckResourceAttr("spotlight_harness_profile.careful", "permissions.ask.#", "0"),
					resource.TestCheckResourceAttr("spotlight_line.delivery", "station.1.profile", k.profile),
					resource.TestCheckResourceAttr("spotlight_line.delivery", "station.1.runners.0", "tf-acc-gpu"),
					resource.TestCheckResourceAttr("spotlight_line.delivery", "runners.0", "tf-acc-vpc"),
					resource.TestCheckResourceAttr("spotlight_line.delivery", "reviews.skip.authors.#", "2"),
				),
			},
			{
				Config:           adopted(cp.workspace) + profiledConfig(k, `"read"`, "2.1.270"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config: adopted(cp.workspace) + profiledConfig(k, `"read", "execute(npm test *)"`, "v2.1.271"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("spotlight_harness_profile.careful", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("spotlight_line.delivery", plancheck.ResourceActionUpdate),
				}},
			},
			{
				Config:           adopted(cp.workspace) + profiledConfig(k, `"read", "execute(npm test *)"`, "v2.1.271"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      "spotlight_harness_profile.careful",
				ImportState:       true,
				ImportStateId:     importID(id, k.team+"/"+k.profile),
				ImportStateVerify: true,
			},
			{
				ResourceName:            "spotlight_line.delivery",
				ImportState:             true,
				ImportStateId:           importID(id, k.team+"/"+k.line),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"station.1.harness_version"},
			},
			{
				Config: released(cp.workspace) + profiledConfig(k, `"read", "execute(npm test *)"`, "v2.1.271"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					forgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction("spotlight_harness_profile.careful", plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}
