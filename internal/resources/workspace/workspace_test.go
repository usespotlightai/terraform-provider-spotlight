package workspace_test

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/providertest"
)

const address = "spotlight_workspace.acme"

func config(name, slug string) string {
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
		ProtoV6ProviderFactories: providertest.Factories(fake),
		CheckDestroy:             providertest.NoneLeft(fake, "workspace"),
		Steps: []resource.TestStep{
			{
				Config: config("Acme", "acme"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(address, "id"),
					resource.TestCheckResourceAttr(address, "name", "Acme"),
					resource.TestCheckResourceAttr(address, "slug", "acme"),
					resource.TestCheckResourceAttr(address, "address", "acme"),
					resource.TestCheckResourceAttr(address, "version", "1"),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     "acme",
				ImportStateVerify: true,
			},
			{
				Config: config("Acme Corp", "acme"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "name", "Acme Corp"),
					resource.TestCheckResourceAttr(address, "version", "2"),
				),
			},
			{
				Config: config("Acme Corp", "acme-corp"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.TestCheckResourceAttr(address, "address", "acme-corp"),
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("workspace", "acme-corp"), func(fields map[string]any) { fields["name"] = "Renamed elsewhere" })
				},
				Config: config("Acme Corp", "acme-corp"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr(address, "name", "Acme Corp"),
			},
			{
				PreConfig: func() { fake.Forget(fake.IDOf("workspace", "acme-corp")) },
				Config:    config("Acme Corp", "acme-corp"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionCreate)},
				},
			},
		},
	})
}

func TestWorkspaceCreateSurvivesAFlakyControlPlane(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig: func() { fake.FailNext(http.StatusServiceUnavailable, http.StatusBadGateway) },
				Config:    config("Acme", "acme"),
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
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config:        config("Acme", "acme"),
				ResourceName:  address,
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
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config:      config("", "acme"),
				ExpectError: regexp.MustCompile(`(?s)with spotlight_workspace\.acme,.*name: must not be empty`),
			},
		},
	})
}

func TestAWorkspaceATokenMayNotCreateIsToBeImported(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.FailNext(http.StatusForbidden) },
				Config:      config("Acme", "acme"),
				ExpectError: regexp.MustCompile(`may not open a new\s+one(?s:.*)import it\s+rather than creating it(?s:.*)to = spotlight_workspace\.<name>`),
			},
		},
	})
}
