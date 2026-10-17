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

const hookAddress = "spotlight_hook.grafana_reads"

const hookIntegrations = `
resource "spotlight_integration" "grafana" {
  team_id = spotlight_team.core.id
  definition = {
    integration = "grafana"
    tools = {
      search = { description = "Search the dashboards." }
    }
  }
}

resource "spotlight_integration" "slack" {
  team_id = spotlight_team.core.id
  definition = {
    integration = "slack"
    tools = {
      post-message = { description = "Post a message to a channel." }
    }
  }
}
`

func hookConfig(key, when, text, secret string, version int) string {
	return acctest.Workspace + hookIntegrations + fmt.Sprintf(`
resource "spotlight_hook" "grafana_reads" {
  team_id = spotlight_team.core.id
  key     = %q
  on      = "tool.used"
  match = {
    tools = [spotlight_integration.grafana.tools["search"]]
    lines = ["delivery"]
  }
  when = %q
  vars = { channel = "C024BE91L" }
  steps = [
    {
      name = "notify"
      call = spotlight_integration.slack.tools["post-message"]
      with = { channel = "$${{ vars.channel }}", text = %q, unfurl = false }
    },
    {
      name = "audit"
      request = {
        url  = "https://audit.acme.dev/hooks"
        body = { query = "$${{ event.inputs.query }}", attempts = 3 }
      }
    },
  ]
  signing = {
    secret_wo         = %q
    secret_wo_version = %d
  }
}
`, key, when, text, secret, version)
}

const (
	hookSoundWhen = `event.inputs.query != ''`
	hookSoundText = `Agent searched Grafana: $${{ event.inputs.query }}`
)

func hookImportID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/core/grafana-reads", nil
}

func hookSecret(fake *fakeapi.Server, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := fake.WriteOnly(fake.IDOf("hook", "core/grafana-reads")); got != want {
			return fmt.Errorf("the hook signs with %q, want %q", got, want)
		}

		return nil
	}
}

func hookSigningSecret(body map[string]any) (any, bool) {
	signing, _ := body["signing"].(map[string]any)
	secret, ok := signing["secret"]

	return secret, ok
}

func TestHookLifecycle(t *testing.T) {
	fake := fakeapi.New(t)
	config := hookConfig("grafana-reads", hookSoundWhen, hookSoundText, provisionFirst, 1)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "hook", "integration", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(hookAddress, tfjsonpath.New("address"), knownvalue.StringExact("core/grafana-reads")),
					statecheck.ExpectKnownValue(hookAddress, tfjsonpath.New("match").AtMapKey("tools"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("grafana.search")})),
					statecheck.ExpectKnownValue(hookAddress, tfjsonpath.New("match").AtMapKey("paths"), knownvalue.SetExact([]knownvalue.Check{})),
					statecheck.ExpectKnownValue(hookAddress, tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("call"), knownvalue.StringExact("slack.post-message")),
					statecheck.ExpectKnownValue(hookAddress, tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("with").AtMapKey("text"),
						knownvalue.StringExact("Agent searched Grafana: ${{ event.inputs.query }}")),
					statecheck.ExpectKnownValue(hookAddress, tfjsonpath.New("signing").AtMapKey("secret_wo"), knownvalue.Null()),
					statecheck.ExpectKnownValue(hookAddress, tfjsonpath.New("signing").AtMapKey("secret_wo_version"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(hookAddress, tfjsonpath.New("failure"), knownvalue.StringExact("continue")),
					statecheck.ExpectKnownValue(hookAddress, tfjsonpath.New("timeout_seconds"), knownvalue.Int64Exact(10)),
					statecheck.ExpectKnownValue("spotlight_integration.slack", tfjsonpath.New("tools"),
						knownvalue.MapExact(map[string]knownvalue.Check{"post-message": knownvalue.StringExact("slack.post-message")})),
				},
				Check: resource.ComposeAggregateTestCheckFunc(hookSecret(fake, provisionFirst), acctest.NothingContains(acctest.Value(provisionFirst))),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
			{
				ResourceName:      hookAddress,
				ImportState:       true,
				ImportStateIdFunc: hookImportID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("hook", "core/grafana-reads"), func(fields map[string]any) { fields["failure"] = "block" })
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(hookAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(hookAddress, tfjsonpath.New("failure"), knownvalue.StringExact("continue")),
				},
				Check: sentTimes(fake, hookSigningSecret, 1, "putting back what changed sends no secret"),
			},
			{
				Config: hookConfig("grafana-reads", hookSoundWhen, hookSoundText, provisionSecond, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(hookAddress, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					hookSecret(fake, provisionSecond),
					sentTimes(fake, hookSigningSecret, 2, "a new secret_wo_version sends the secret again"),
					acctest.NothingContains(acctest.Value(provisionFirst), acctest.Value(provisionSecond)),
				),
			},
			{
				Config: hookConfig("grafana-searches", hookSoundWhen, hookSoundText, provisionSecond, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(hookAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(hookAddress, tfjsonpath.New("address"), knownvalue.StringExact("core/grafana-searches")),
				},
			},
		},
	})
}

const hookMigrations = `
resource "spotlight_hook" "migrations" {
  team_id = spotlight_team.core.id
  key     = "migrations-need-a-dba"
  on      = "pull-request.opening"
  match = {
    paths = ["db/migrations/**"]
  }
  vars = { migrations = "db/migrations/" }
  steps = [
    {
      name = "review"
      request = {
        url     = "https://dba.acme.dev/review"
        headers = { x-team = "core" }
        body    = "$${{ {'pr': event.title, 'files': event.changed.filter(path, path.startsWith(vars.migrations))} }}"
        from    = "network"
      }
    },
  ]
  signing = {
    oidc = { audience = "https://dba.acme.dev" }
  }
  failure         = "block"
  timeout_seconds = 20
}
`

func TestHookImportedAsExportedHasNoDiff(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + hookMigrations,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("spotlight_hook.migrations", tfjsonpath.New("signing").AtMapKey("oidc").AtMapKey("audience"), knownvalue.StringExact("https://dba.acme.dev")),
					statecheck.ExpectKnownValue("spotlight_hook.migrations", tfjsonpath.New("when"), knownvalue.Null()),
				},
			},
			{Config: acctest.Workspace + hookMigrations, PlanOnly: true},
			{
				ResourceName: "spotlight_hook.migrations",
				ImportState:  true,
				ImportStateIdFunc: func(state *terraform.State) (string, error) {
					return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/core/migrations-need-a-dba", nil
				},
				ImportStateVerify: true,
			},
		},
	})
}

func TestHookBadExpressionFailsPlanAtItsAttribute(t *testing.T) {
	fake := fakeapi.New(t)

	literal := func(when, text string) string {
		return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_hook" "grafana_reads" {
  team_id = spotlight_team.core.id
  key     = "grafana-reads"
  on      = "tool.used"
  when    = %q
  steps = [
    {
      name = "notify"
      call = "slack.post-message"
      with = { text = %q }
    },
  ]
}
`, when, text)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      literal(`event.inptus.query != ''`, hookSoundText),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Invalid expression in when.*No such key: inptus.*Did you mean event\.inputs\?`),
			},
			{
				Config:      literal(hookSoundWhen, `Agent searched $${{ event.inputs.query }} as $${{ event.tol }}`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Invalid expression in steps\[0\]\.with\.text.*No such key: tol.*Did you mean event\.tool\?`),
			},
			{
				Config:      literal(`"yes"`, hookSoundText),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`must return a bool, not a string`),
			},
		},
	})

	if fake.Count("hook") != 0 {
		t.Fatal("a plan that failed its checks created a hook")
	}
}

func TestHookRenamedToolFailsPlanAtTheReference(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + hookIntegrations + `
resource "spotlight_hook" "grafana_reads" {
  team_id = spotlight_team.core.id
  key     = "grafana-reads"
  on      = "tool.used"
  match   = { tools = [spotlight_integration.grafana.tools["query"]] }
  steps = [
    { name = "notify", call = spotlight_integration.slack.tools["post-message"] },
  ]
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Invalid index.*tools\["query"\]`),
			},
		},
	})
}

func TestHookSigningAndStepsAreCheckedAtPlan(t *testing.T) {
	fake := fakeapi.New(t)

	hook := func(signing, steps string) string {
		return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_hook" "grafana_reads" {
  team_id = spotlight_team.core.id
  key     = "grafana-reads"
  on      = "run.ended"
  signing = %s
  steps   = %s
}
`, signing, steps)
	}

	sound := `[{ name = "audit", request = { url = "https://audit.acme.dev" } }]`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      hook(`{ secret_wo_version = 1, oidc = { audience = "https://audit.acme.dev" } }`, sound),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`signs with an HMAC secret or with an OIDC token, not both`),
			},
			{
				Config:      hook(fmt.Sprintf(`{ secret_wo = %q }`, provisionFirst), sound),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`signing\.secret_wo_version: says when to send secret_wo again`),
			},
			{
				Config:      hook(`{ secret_wo = "whsec_x", secret_wo_version = 0 }`, sound),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)signing\.secret_wo_version.*must be at least 1`),
			},
			{
				Config:      hook(`null`, `{ name = "audit" }`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`steps: is the steps the hook takes, in order`),
			},
			{
				Config:      hook(`null`, sound),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`signing: is missing; a hook with a step that sends a request signs it`),
			},
			{
				Config:      hook(`{ oidc = { audience = "https://audit.acme.dev" } }`, `[{ name = "audit", call = "slack.post-message", request = { url = "https://audit.acme.dev" } }]`),
				ExpectError: regexp.MustCompile(`(?s)steps\[0\]: names exactly one of call and\s+request`),
			},
		},
	})
}

func TestHookStepsFromAListOfObjectsSendNoNulls(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + `
variable "steps" {
  type = list(object({
    name    = string
    call    = optional(string)
    with    = optional(map(string))
    request = optional(object({ url = string, body = optional(string) }))
  }))
  default = [
    { name = "notify", call = "slack.post-message", with = { text = "$${{ event.tol }}" } },
    { name = "audit", request = { url = "https://audit.acme.dev" } },
  ]
}

resource "spotlight_hook" "audit" {
  team_id = spotlight_team.core.id
  key     = "audit"
  on      = "tool.used"
  steps   = var.steps
  signing = { oidc = { audience = "https://audit.acme.dev" } }
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Invalid expression in steps\[0\]\.with\.text.*No such key: tol`),
			},
			{
				Config: acctest.Workspace + `
variable "steps" {
  type = list(object({
    name    = string
    call    = optional(string)
    request = optional(object({ url = string, body = optional(string) }))
  }))
  default = [
    { name = "notify", call = "slack.post-message" },
    { name = "audit", request = { url = "https://audit.acme.dev" } },
  ]
}

resource "spotlight_hook" "audit" {
  team_id = spotlight_team.core.id
  key     = "audit"
  on      = "tool.used"
  steps   = var.steps
  signing = { oidc = { audience = "https://audit.acme.dev" } }
}
`,
				Check: func(*terraform.State) error {
					for _, each := range fake.Requests() {
						for _, step := range stepsIn(each.Body) {
							if hasNull(step) || hasNull(step["request"]) {
								return fmt.Errorf("%s %s sent a step that contains null: %v", each.Method, each.Path, step)
							}
						}
					}

					return nil
				},
			},
			{
				Config: acctest.Workspace + `
variable "steps" {
  type = list(object({
    name    = string
    call    = optional(string)
    request = optional(object({ url = string, body = optional(string) }))
  }))
  default = [
    { name = "notify", call = "slack.post-message" },
    { name = "audit", request = { url = "https://audit.acme.dev" } },
  ]
}

resource "spotlight_hook" "audit" {
  team_id = spotlight_team.core.id
  key     = "audit"
  on      = "tool.used"
  steps   = var.steps
  signing = { oidc = { audience = "https://audit.acme.dev" } }
}
`,
				PlanOnly: true,
			},
		},
	})
}

func stepsIn(body map[string]any) []map[string]any {
	var out []map[string]any

	for _, each := range append(listed(body["steps"]), listed(objectIn(body, "definition")["steps"])...) {
		if step, ok := each.(map[string]any); ok {
			out = append(out, step)
		}
	}

	return out
}

func listed(value any) []any {
	out, _ := value.([]any)

	return out
}

func objectIn(body map[string]any, name string) map[string]any {
	out, _ := body[name].(map[string]any)

	return out
}

func hasNull(value any) bool {
	object, _ := value.(map[string]any)
	for _, value := range object {
		if value == nil {
			return true
		}
	}

	return false
}

func TestHookSkipsExpressionsNotKnownYet(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + `
resource "spotlight_hook" "audit" {
  team_id = spotlight_team.core.id
  key     = "audit"
  on      = "run.ended"
  when    = "task.title == '${spotlight_team.core.id}'"
  steps = [
    { name = "audit", request = { url = "https://audit.acme.dev/${spotlight_team.core.id}/$${{ task.title }}" } },
  ]
  signing = { oidc = { audience = "https://audit.acme.dev" } }
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("spotlight_hook.audit", tfjsonpath.New("version"), knownvalue.Int64Exact(1)),
				},
			},
		},
	})
}

func TestHookImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            acctest.Workspace + hookMigrations,
				ConfigStateChecks: acctest.ExpectIdentity("spotlight_hook.migrations"),
			},
			acctest.ImportedByIdentity("spotlight_hook.migrations"),
		},
	})
}

func hookAccConfig(k acctest.Keys, text string) string {
	return fmt.Sprintf(`
resource "spotlight_team" "acc" {
  workspace_id = spotlight_workspace.acc.id
  key          = %[1]q
  name         = "Terraform acceptance"
}

resource "spotlight_integration" "linear" {
  team_id = spotlight_team.acc.id
  definition = {
    integration = %[2]q
    actions = {
      comment = {
        request = {
          url  = "https://api.linear.app/graphql"
          body = { query = "$${{ inputs.body }}" }
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

resource "spotlight_hook" "audit" {
  team_id = spotlight_team.acc.id
  key     = %[3]q
  on      = "tool.used"
  match   = { tools = [spotlight_integration.linear.tools["comment"]] }
  when    = "event.inputs.body != ''"
  steps = [
    {
      name = "note"
      call = spotlight_integration.linear.tools["comment"]
      with = { body = %[4]q }
    },
    {
      name    = "audit"
      request = { url = "https://audit.acme.dev/hooks", body = { tool = "$${{ event.tool }}" }, from = "network" }
    },
  ]
  signing = {
    secret_wo         = %[5]q
    secret_wo_version = 1
  }
}
`, k.Team, k.Integration, k.Hook, text, acctest.RandomSigningSecret())
}

func TestAccHook(t *testing.T) {
	cp := acctest.PreCheck(t)
	k := acctest.RandomKeys()
	cp.Sweep(t, k)

	id := cp.Workspace.ID
	config := acctest.AdoptedWorkspace(cp.Workspace) + hookAccConfig(k, "Commented: $${{ event.inputs.body }}")

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_hook.audit", "address", k.Team+"/"+k.Hook),
					resource.TestCheckResourceAttr("spotlight_integration.linear", "tools.comment", k.Integration+".comment"),
					resource.TestCheckResourceAttr("spotlight_hook.audit", "signing.secret_wo_version", "1"),
				),
			},
			{
				Config:           config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      "spotlight_hook.audit",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(id, k.Team+"/"+k.Hook),
				ImportStateVerify: true,
			},
			{
				Config:      acctest.AdoptedWorkspace(cp.Workspace) + hookAccConfig(k, "Commented by $${{ event.tol }}"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Invalid expression in steps\[0\]\.with\.body`),
			},
		},
	})
}
