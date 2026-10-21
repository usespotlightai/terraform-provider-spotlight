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

const oidcTrustAddress = "spotlight_oidc_trust.github"

func oidcTrustConfig(key, subject, scopes, more string) string {
	return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_oidc_trust" "github" {
  workspace_id = spotlight_workspace.acme.id
  key          = %q
  issuer       = "https://token.actions.githubusercontent.com"
  subject      = %q
  scopes       = [%s]
  %s
}
`, key, subject, scopes, more)
}

func oidcTrustImportID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/github-main", nil
}

func TestOidcTrustLifecycle(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "oidc-trust", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: oidcTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"configure", "secrets"`, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(oidcTrustAddress, tfjsonpath.New("address"), knownvalue.StringExact("github-main")),
					statecheck.ExpectKnownValue(oidcTrustAddress, tfjsonpath.New("audience"), knownvalue.StringExact("spotlight")),
					statecheck.ExpectKnownValue(oidcTrustAddress, tfjsonpath.New("claims"), knownvalue.MapExact(map[string]knownvalue.Check{})),
					statecheck.ExpectKnownValue(oidcTrustAddress, tfjsonpath.New("lifetime_minutes"), knownvalue.Int64Exact(15)),
					statecheck.ExpectKnownValue(oidcTrustAddress, tfjsonpath.New("created_by"), knownvalue.StringExact(fakeapi.Admin)),
					statecheck.ExpectKnownValue(oidcTrustAddress, tfjsonpath.New("scopes"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("configure"), knownvalue.StringExact("secrets"),
					})),
				},
			},
			{
				Config:   oidcTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"secrets", "configure"`, ""),
				PlanOnly: true,
			},
			{
				Config: oidcTrustConfig("github-main", "repo:acme/platform:environment:*", `"configure", "secrets", "runners", "trusts"`,
					`claims           = { repository = "acme/platform" }
  lifetime_minutes = 30`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(oidcTrustAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(oidcTrustAddress, tfjsonpath.New("version"), knownvalue.Int64Exact(2)),
					statecheck.ExpectKnownValue(oidcTrustAddress, tfjsonpath.New("claims"), knownvalue.MapExact(map[string]knownvalue.Check{
						"repository": knownvalue.StringExact("acme/platform"),
					})),
					statecheck.ExpectKnownValue(oidcTrustAddress, tfjsonpath.New("lifetime_minutes"), knownvalue.Int64Exact(30)),
				},
			},
			{
				ResourceName:      oidcTrustAddress,
				ImportState:       true,
				ImportStateIdFunc: oidcTrustImportID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("oidc-trust", "github-main"), func(fields map[string]any) { fields["scopes"] = []any{"plan"} })
				},
				Config: oidcTrustConfig("github-main", "repo:acme/platform:environment:*", `"configure", "secrets", "runners", "trusts"`,
					`claims           = { repository = "acme/platform" }
  lifetime_minutes = 30`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(oidcTrustAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(oidcTrustAddress, tfjsonpath.New("scopes"), knownvalue.SetSizeExact(4)),
				},
			},
			{
				Config: oidcTrustConfig("github-pull-requests", "repo:acme/platform:pull_request", `"plan"`, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(oidcTrustAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
			},
		},
	})
}

func TestOidcTrustWritesNeedTheTrustsScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.Grant("configure", "secrets", "runners") },
				Config:      oidcTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"plan"`, ""),
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
				Config:      oidcTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"configure", "secrets"`, ""),
				ExpectError: regexp.MustCompile(`(?s)not\s+change\s+it.*lacks\s+the\s+trusts\s+scope.*or\s+a\s+scope\s+the\s+trust\s+grants`),
			},
			{
				PreConfig: func() { fake.Grant("configure", "secrets", "trusts") },
				Config:    oidcTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"configure", "secrets"`, ""),
			},
			{
				PreConfig:   func() { fake.Grant("configure", "trusts") },
				Config:      oidcTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"configure"`, ""),
				ExpectError: regexp.MustCompile(`(?s)not\s+change\s+it`),
			},
			{
				PreConfig: func() { fake.Grant("configure", "secrets", "trusts") },
				Config:    oidcTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"configure"`, ""),
				Check:     resource.TestCheckResourceAttr(oidcTrustAddress, "scopes.#", "1"),
			},
		},
	})
}

func TestOidcTrustIsCheckedWhenPlanned(t *testing.T) {
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
						Config:      oidcTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", each.scopes, each.more),
						PlanOnly:    true,
						ExpectError: each.rejected,
					},
				},
			})

			if created := fake.Count("oidc-trust"); created != 0 {
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
				Config:      oidcTrustConfig("github-main", "repo:*", `"plan"`, ""),
				ExpectError: regexp.MustCompile(`(?s)subject: names its owner before any \*`),
			},
			{
				Config: oidcTrustConfig("github-main", "repo:acme/*", `"plan"`, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(oidcTrustAddress, tfjsonpath.New("subject"), knownvalue.StringExact("repo:acme/*")),
				},
			},
		},
	})
}

func TestOidcTrustImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            oidcTrustConfig("github-main", "repo:acme/platform:ref:refs/heads/main", `"plan"`, ""),
				ConfigStateChecks: acctest.ExpectIdentity(oidcTrustAddress),
			},
			acctest.ImportedByIdentity(oidcTrustAddress),
		},
	})
}

func oidcTrustAccConfig(k acctest.Keys, scopes string) string {
	return fmt.Sprintf(`
resource "spotlight_oidc_trust" "github" {
  workspace_id = local.workspace_id
  key          = %[1]q
  issuer       = "https://token.actions.githubusercontent.com"
  subject      = "repo:tf-acc/%[1]s:ref:refs/heads/main"
  scopes       = [%[2]s]
}
`, k.OidcTrust, scopes)
}

func TestAccOidcTrust(t *testing.T) {
	cp := acctest.PreCheck(t)
	k := acctest.RandomKeys()
	cp.Sweep(t, k)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config: acctest.AdoptedWorkspace(cp.Workspace) + oidcTrustAccConfig(k, `"plan"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(oidcTrustAddress, "address", k.OidcTrust),
					resource.TestCheckResourceAttr(oidcTrustAddress, "audience", "spotlight"),
					resource.TestCheckResourceAttr(oidcTrustAddress, "claims.%", "0"),
					resource.TestCheckResourceAttr(oidcTrustAddress, "lifetime_minutes", "15"),
					resource.TestCheckResourceAttrSet(oidcTrustAddress, "created_by"),
				),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + oidcTrustAccConfig(k, `"plan"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config: acctest.AdoptedWorkspace(cp.Workspace) + oidcTrustAccConfig(k, `"configure", "secrets"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(oidcTrustAddress, plancheck.ResourceActionUpdate),
				}},
				Check: resource.TestCheckResourceAttr(oidcTrustAddress, "scopes.#", "2"),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + oidcTrustAccConfig(k, `"secrets", "configure"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      oidcTrustAddress,
				ImportState:       true,
				ImportStateId:     acctest.ImportID(cp.Workspace.ID, k.OidcTrust),
				ImportStateVerify: true,
			},
			{
				Config: acctest.ReleasedWorkspace(cp.Workspace) + oidcTrustAccConfig(k, `"secrets", "configure"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					acctest.ExpectForgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction(oidcTrustAddress, plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}
