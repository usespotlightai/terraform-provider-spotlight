package provider_test

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

const workspaceAddress = "spotlight_workspace.acme"

func workspaceConfig(name, slug string) string {
	return fmt.Sprintf(`
resource "spotlight_workspace" "acme" {
  name = %q
  slug = %q
}
`, name, slug)
}

func TestWorkspaceLifecycle(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "workspace"),
		Steps: []resource.TestStep{
			{
				Config: workspaceConfig("Acme", "acme"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(workspaceAddress, "id"),
					resource.TestCheckResourceAttr(workspaceAddress, "name", "Acme"),
					resource.TestCheckResourceAttr(workspaceAddress, "slug", "acme"),
					resource.TestCheckResourceAttr(workspaceAddress, "address", "acme"),
					resource.TestCheckResourceAttr(workspaceAddress, "version", "1"),
				),
			},
			{
				ResourceName:      workspaceAddress,
				ImportState:       true,
				ImportStateId:     "acme",
				ImportStateVerify: true,
			},
			{
				Config: workspaceConfig("Acme Corp", "acme"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceAddress, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(workspaceAddress, "name", "Acme Corp"),
					resource.TestCheckResourceAttr(workspaceAddress, "version", "2"),
				),
			},
			{
				Config: workspaceConfig("Acme Corp", "acme-corp"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceAddress, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.TestCheckResourceAttr(workspaceAddress, "address", "acme-corp"),
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("workspace", "acme-corp"), func(fields map[string]any) { fields["name"] = "Renamed elsewhere" })
				},
				Config: workspaceConfig("Acme Corp", "acme-corp"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(workspaceAddress, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr(workspaceAddress, "name", "Acme Corp"),
			},
			{
				PreConfig: func() { fake.Forget(fake.IDOf("workspace", "acme-corp")) },
				Config:    workspaceConfig("Acme Corp", "acme-corp"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceAddress, plancheck.ResourceActionCreate)},
				},
			},
		},
	})
}

func TestWorkspaceCreateSurvivesAFlakyControlPlane(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig: func() { fake.FailNext(http.StatusServiceUnavailable, http.StatusBadGateway) },
				Config:    workspaceConfig("Acme", "acme"),
				Check: func(*terraform.State) error {
					if created := fake.Count("workspace"); created != 1 {
						return fmt.Errorf("%d workspaces created, want 1", created)
					}

					keys := map[string]bool{}
					for _, each := range fake.Requests() {
						if each.Method == http.MethodPost {
							keys[each.Headers.Get("Idempotency-Key")] = true
						}
					}
					if len(keys) != 1 {
						return fmt.Errorf("retries used %d idempotency keys, want 1", len(keys))
					}

					return nil
				},
			},
		},
	})
}

func TestWorkspaceImportUnknown(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:        workspaceConfig("Acme", "acme"),
				ResourceName:  workspaceAddress,
				ImportState:   true,
				ImportStateId: "nowhere",
				ExpectError:   regexp.MustCompile(`Could not import the workspace at nowhere`),
			},
		},
	})
}

func TestWorkspaceRefusalNamesTheAttribute(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      workspaceConfig("", "acme"),
				ExpectError: regexp.MustCompile(`(?s)with spotlight_workspace\.acme,.*name: must not be empty`),
			},
		},
	})
}

func TestAWorkspaceATokenMayNotCreateIsToBeImported(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.FailNext(http.StatusForbidden) },
				Config:      workspaceConfig("Acme", "acme"),
				ExpectError: regexp.MustCompile(`may not open a new\s+one(?s:.*)import it\s+rather than creating it(?s:.*)to = spotlight_workspace\.<name>`),
			},
		},
	})
}

func TestWorkspaceImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            workspaceConfig("Acme", "acme"),
				ConfigStateChecks: acctest.ExpectIdentity(workspaceAddress),
			},
			acctest.ImportedByIdentity(workspaceAddress),
		},
	})
}
