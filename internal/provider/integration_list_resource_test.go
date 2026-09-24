package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededIntegrations(t *testing.T, seeded acctest.Seeded) (core, platform acctest.Listed) {
	t.Helper()

	held := func(team client.Team, key string) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateIntegration(t.Context(), team.ID, client.Definition{"integration": key})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	held(seeded.OtherCore, "linear")

	return held(seeded.Core, "linear"), held(seeded.Platform, "github")
}

func TestIntegrationListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	core, platform := seededIntegrations(t, seeded)

	all := acctest.List(t, fake, "spotlight_integration", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{core, platform}; !slices.Equal(all, want) {
		t.Errorf("listed %v, want %v", all, want)
	}

	held := acctest.List(t, fake, "spotlight_integration", map[string]string{"workspace_id": seeded.Workspace.ID, "team_id": seeded.Core.ID})
	if want := []acctest.Listed{core}; !slices.Equal(held, want) {
		t.Errorf("listed %v in the core team, want %v", held, want)
	}
}

func TestIntegrationListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	core, platform := seededIntegrations(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query: true,
				Config: acctest.QueryConfig(
					acctest.ListBlock("spotlight_integration", "all", map[string]string{"workspace_id": seeded.Workspace.ID}),
					acctest.ListBlock("spotlight_integration", "core", map[string]string{"workspace_id": seeded.Workspace.ID, "team_id": seeded.Core.ID}),
				),
				QueryResultChecks: slices.Concat(
					acctest.ExpectListed("spotlight_integration.all", core, platform),
					acctest.ExpectListed("spotlight_integration.core", core),
				),
			},
		},
	})
}
