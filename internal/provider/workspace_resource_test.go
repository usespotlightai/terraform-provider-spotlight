package provider_test

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
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
				PreConfig: func() { fake.Delete(fake.IDOf("workspace", "acme-corp")) },
				Config:    workspaceConfig("Acme Corp", "acme-corp"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceAddress, plancheck.ResourceActionCreate)},
				},
			},
		},
	})
}

func workspaceHostsConfig(hosts string) string {
	return fmt.Sprintf(`
resource "spotlight_workspace" "acme" {
  name           = "Acme"
  slug           = "acme"
  outbound_hosts = [%s]
}
`, hosts)
}

func TestWorkspaceOutboundHosts(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: workspaceConfig("Acme", "acme"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(workspaceAddress, tfjsonpath.New("outbound_hosts"), knownvalue.SetExact([]knownvalue.Check{})),
				},
			},
			{
				Config: workspaceHostsConfig(`"api.example.com", "*.acme.io"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(workspaceAddress, tfjsonpath.New("outbound_hosts"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("api.example.com"),
						knownvalue.StringExact("*.acme.io"),
					})),
				},
				Check: func(*terraform.State) error {
					var sent []string
					for _, each := range fake.Requests() {
						if each.Method == http.MethodPut {
							listed, _ := each.Body["outboundHosts"].([]any)
							sent = sent[:0]
							for _, host := range listed {
								text, _ := host.(string)
								sent = append(sent, text)
							}
						}
					}
					slices.Sort(sent)
					if !slices.Equal(sent, []string{"*.acme.io", "api.example.com"}) {
						return fmt.Errorf("the update sent outbound hosts %v", sent)
					}

					return nil
				},
			},
			{
				ResourceName:      workspaceAddress,
				ImportState:       true,
				ImportStateId:     "acme",
				ImportStateVerify: true,
			},
			{
				Config: workspaceHostsConfig(`"API.Example.com.", "*.acme.io"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceAddress, plancheck.ResourceActionUpdate)},
				},
			},
			{
				Config:   workspaceHostsConfig(`"API.Example.com.", "*.acme.io"`),
				PlanOnly: true,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("workspace", "acme"), func(fields map[string]any) { fields["outboundHosts"] = []any{"elsewhere.example.com"} })
				},
				Config: workspaceHostsConfig(`"api.example.com", "*.acme.io"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(workspaceAddress, tfjsonpath.New("outbound_hosts"), knownvalue.SetSizeExact(2)),
				},
			},
			{
				Config: workspaceConfig("Acme", "acme"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(workspaceAddress, tfjsonpath.New("outbound_hosts"), knownvalue.SetExact([]knownvalue.Check{})),
				},
			},
		},
	})
}

func TestWorkspaceOutboundHostProblemNamesTheAttribute(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      workspaceHostsConfig(`"*"`),
				ExpectError: regexp.MustCompile(`(?s)with spotlight_workspace\.acme,.*outbound_hosts: Allowed host "\*" permits every\s+host`),
			},
			{
				Config:      workspaceHostsConfig(`"bücher.example"`),
				ExpectError: regexp.MustCompile(`(?s)with spotlight_workspace\.acme,.*outbound_hosts: Allowed host\s+"bücher\.example"\s+has\s+a\s+character\s+a\s+host\s+name\s+cannot\s+have`),
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

func TestWorkspaceProblemNamesTheAttribute(t *testing.T) {
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
				ExpectError: regexp.MustCompile(`may not create a new\s+one(?s:.*)import it\s+rather than creating it(?s:.*)to = spotlight_workspace\.<name>`),
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
