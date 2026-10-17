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

const workspaceTokenTrustAddress = "spotlight_workspace_token_trust.github"

func workspaceTokenTrustConfig(key, subject, scopes, more string) string {
	return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_workspace_token_trust" "github" {
  workspace_id = spotlight_workspace.acme.id
  key          = %q
  issuer       = "https://token.actions.githubusercontent.com"
  subject      = %q
  scopes       = [%s]
  %s
}
`, key, subject, scopes, more)
}

func workspaceTokenTrustImportID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/github-main", nil
}

func TestWorkspaceTokenTrustLifecycle(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "workspace-token-trust", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: workspaceTokenTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"configure", "secrets"`, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(workspaceTokenTrustAddress, tfjsonpath.New("address"), knownvalue.StringExact("github-main")),
					statecheck.ExpectKnownValue(workspaceTokenTrustAddress, tfjsonpath.New("audience"), knownvalue.StringExact("spotlight")),
					statecheck.ExpectKnownValue(workspaceTokenTrustAddress, tfjsonpath.New("claims"), knownvalue.MapExact(map[string]knownvalue.Check{})),
					statecheck.ExpectKnownValue(workspaceTokenTrustAddress, tfjsonpath.New("lifetime_minutes"), knownvalue.Int64Exact(15)),
					statecheck.ExpectKnownValue(workspaceTokenTrustAddress, tfjsonpath.New("created_by"), knownvalue.StringExact(fakeapi.Admin)),
					statecheck.ExpectKnownValue(workspaceTokenTrustAddress, tfjsonpath.New("scopes"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("configure"), knownvalue.StringExact("secrets"),
					})),
				},
			},
			{
				Config:   workspaceTokenTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"secrets", "configure"`, ""),
				PlanOnly: true,
			},
			{
				Config: workspaceTokenTrustConfig("github-main", "repo:acme/platform:environment:*", `"configure", "secrets", "runners", "trusts"`,
					`claims           = { repository = "acme/platform" }
  lifetime_minutes = 30`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceTokenTrustAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(workspaceTokenTrustAddress, tfjsonpath.New("version"), knownvalue.Int64Exact(2)),
					statecheck.ExpectKnownValue(workspaceTokenTrustAddress, tfjsonpath.New("claims"), knownvalue.MapExact(map[string]knownvalue.Check{
						"repository": knownvalue.StringExact("acme/platform"),
					})),
					statecheck.ExpectKnownValue(workspaceTokenTrustAddress, tfjsonpath.New("lifetime_minutes"), knownvalue.Int64Exact(30)),
				},
			},
			{
				ResourceName:      workspaceTokenTrustAddress,
				ImportState:       true,
				ImportStateIdFunc: workspaceTokenTrustImportID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("workspace-token-trust", "github-main"), func(fields map[string]any) { fields["scopes"] = []any{"plan"} })
				},
				Config: workspaceTokenTrustConfig("github-main", "repo:acme/platform:environment:*", `"configure", "secrets", "runners", "trusts"`,
					`claims           = { repository = "acme/platform" }
  lifetime_minutes = 30`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceTokenTrustAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(workspaceTokenTrustAddress, tfjsonpath.New("scopes"), knownvalue.SetSizeExact(4)),
				},
			},
			{
				Config: workspaceTokenTrustConfig("github-pull-requests", "repo:acme/platform:pull_request", `"plan"`, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceTokenTrustAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
			},
		},
	})
}

func TestWorkspaceTokenTrustWritesNeedTheTrustsScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.Grant("configure", "secrets", "runners") },
				Config:      workspaceTokenTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"plan"`, ""),
				ExpectError: regexp.MustCompile(`(?s)lacks the trusts scope`),
			},
		},
	})
}

func TestATokenGivesATrustOnlyScopesItHas(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.Grant("configure", "trusts") },
				Config:      workspaceTokenTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"configure", "secrets"`, ""),
				ExpectError: regexp.MustCompile(`(?s)not\s+change\s+it.*lacks\s+the\s+trusts\s+scope.*or\s+a\s+scope\s+the\s+trust\s+grants`),
			},
			{
				PreConfig: func() { fake.Grant("configure", "secrets", "trusts") },
				Config:    workspaceTokenTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"configure", "secrets"`, ""),
			},
			{
				PreConfig:   func() { fake.Grant("configure", "trusts") },
				Config:      workspaceTokenTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"configure"`, ""),
				ExpectError: regexp.MustCompile(`(?s)not\s+change\s+it`),
			},
			{
				PreConfig: func() { fake.Grant("configure", "secrets", "trusts") },
				Config:    workspaceTokenTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"configure"`, ""),
				Check:     resource.TestCheckResourceAttr(workspaceTokenTrustAddress, "scopes.#", "1"),
			},
		},
	})
}

func TestWorkspaceTokenTrustIsCheckedWhenPlanned(t *testing.T) {
	for name, each := range map[string]struct {
		scopes, more string
		rejected     *regexp.Regexp
	}{
		"a lifetime over an hour": {`"plan"`, "lifetime_minutes = 61", regexp.MustCompile(`(?s)lifetime_minutes.*between 1 and 60`)},
		"no scopes":               {"", "", regexp.MustCompile(`(?s)scopes.*at least 1`)},
		"a scope that is not one": {`"administer"`, "", regexp.MustCompile(`(?s)scopes.*administer`)},
	} {
		t.Run(name, func(t *testing.T) {
			fake := fakeapi.New(t)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
				Steps: []resource.TestStep{
					{
						Config:      workspaceTokenTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", each.scopes, each.more),
						PlanOnly:    true,
						ExpectError: each.rejected,
					},
				},
			})

			if created := fake.Count("workspace-token-trust"); created != 0 {
				t.Errorf("%d created", created)
			}
		})
	}
}

func TestAWildcardSubjectNamesItsOwner(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      workspaceTokenTrustConfig("github-main", "repo:*", `"plan"`, ""),
				ExpectError: regexp.MustCompile(`(?s)subject: names its owner before any \*`),
			},
			{
				Config: workspaceTokenTrustConfig("github-main", "repo:acme/*", `"plan"`, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(workspaceTokenTrustAddress, tfjsonpath.New("subject"), knownvalue.StringExact("repo:acme/*")),
				},
			},
		},
	})
}

func TestWorkspaceTokenTrustImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            workspaceTokenTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"plan"`, ""),
				ConfigStateChecks: acctest.ExpectIdentity(workspaceTokenTrustAddress),
			},
			acctest.ImportedByIdentity(workspaceTokenTrustAddress),
		},
	})
}

func workspaceTokenTrustAccConfig(k acctest.Keys, scopes string) string {
	return fmt.Sprintf(`
resource "spotlight_workspace_token_trust" "github" {
  workspace_id = local.workspace_id
  key          = %[1]q
  issuer       = "https://token.actions.githubusercontent.com"
  subject      = "repo:tf-acc/%[1]s:ref:refs/heads/main"
  scopes       = [%[2]s]
}
`, k.TokenTrust, scopes)
}

func TestAccWorkspaceTokenTrust(t *testing.T) {
	cp := acctest.PreCheck(t)
	k := acctest.RandomKeys()
	cp.Sweep(t, k)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config: acctest.AdoptedWorkspace(cp.Workspace) + workspaceTokenTrustAccConfig(k, `"plan"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(workspaceTokenTrustAddress, "address", k.TokenTrust),
					resource.TestCheckResourceAttr(workspaceTokenTrustAddress, "audience", "spotlight"),
					resource.TestCheckResourceAttr(workspaceTokenTrustAddress, "claims.%", "0"),
					resource.TestCheckResourceAttr(workspaceTokenTrustAddress, "lifetime_minutes", "15"),
					resource.TestCheckResourceAttrSet(workspaceTokenTrustAddress, "created_by"),
				),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + workspaceTokenTrustAccConfig(k, `"plan"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config: acctest.AdoptedWorkspace(cp.Workspace) + workspaceTokenTrustAccConfig(k, `"configure", "secrets"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(workspaceTokenTrustAddress, plancheck.ResourceActionUpdate),
				}},
				Check: resource.TestCheckResourceAttr(workspaceTokenTrustAddress, "scopes.#", "2"),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + workspaceTokenTrustAccConfig(k, `"secrets", "configure"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      workspaceTokenTrustAddress,
				ImportState:       true,
				ImportStateId:     acctest.ImportID(cp.Workspace.ID, k.TokenTrust),
				ImportStateVerify: true,
			},
			{
				Config: acctest.ReleasedWorkspace(cp.Workspace) + workspaceTokenTrustAccConfig(k, `"secrets", "configure"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					acctest.ExpectForgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction(workspaceTokenTrustAddress, plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}
