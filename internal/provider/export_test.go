package provider_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/providertest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const (
	exportedWorkspaceID = "00000000-0000-4000-8000-00000000a0a0"
	secretValue         = "lin_exported_never_kept"
)

const requiredProviders = `
terraform {
  required_providers {
    spotlight = {
      source = "usespotlightai/spotlight"
    }
  }
}
`

func TestAnExportAppliesFromScratchAndAdoptsWithNoDiff(t *testing.T) {
	t.Setenv(resource.EnvTfAccProviderNamespace, "usespotlightai")

	fake := fakeapi.New(t)

	api, err := client.New(client.Config{Endpoint: fake.URL, Token: fakeapi.Token})
	if err != nil {
		t.Fatal(err)
	}

	workspace, err := api.Workspaces.Create(t.Context(), "", spotlight.WorkspaceInput{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatal(err)
	}

	dir, err := filepath.Abs("testdata/export")
	if err != nil {
		t.Fatal(err)
	}

	module, err := providertest.ModuleIn(dir)
	if err != nil {
		t.Fatal(err)
	}

	exported := strings.ReplaceAll(module.Config(dir), exportedWorkspaceID, workspace.ID)
	var created []string
	for _, address := range providertest.Addresses(exported) {
		if !strings.HasPrefix(address, "spotlight_workspace.") {
			created = append(created, address)
		}
	}

	fromScratch := providertest.WithoutImports(exported, created...)
	released := requiredProviders + providertest.Released(providertest.Addresses(exported))
	variables := config.Variables{"secret_linear_api_key": config.StringVariable(secretValue)}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config:          fromScratch,
				ConfigVariables: variables,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{providertest.Adopts{Creating: created}},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					holdsOneOfEach(fake),
					providertest.NothingHolds(providertest.Value(secretValue)),
					resource.TestCheckResourceAttr("spotlight_line.core_delivery", "station.1.profile", "careful"),
					resource.TestCheckResourceAttr("spotlight_workspace_runners.acme", "runners.0", "build-1"),
				),
			},
			{
				Config:          exported,
				ConfigVariables: variables,
				PlanOnly:        true,
			},
			{
				Config: released,
			},
			{
				Config:          exported,
				ConfigVariables: variables,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{providertest.Adopts{}},
				},
				Check: holdsOneOfEach(fake),
			},
			{
				Config:          exported,
				ConfigVariables: variables,
				PlanOnly:        true,
			},
		},
	})
}

func holdsOneOfEach(fake *fakeapi.Server) resource.TestCheckFunc {
	return func(*terraform.State) error {
		for kind, want := range map[string]int{
			"workspace": 1, "secret": 2, "runner": 1, "runner-trust": 1, "workspace-runners": 1,
			"team": 1, "line": 1, "skill": 1, "integration": 1, "harness-profile": 1,
		} {
			if got := fake.Count(kind); got != want {
				return fmt.Errorf("Spotlight holds %d of %s, want %d", got, kind, want)
			}
		}

		return nil
	}
}
