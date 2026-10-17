package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededLines(t *testing.T, seeded acctest.Seeded) (core, platform acctest.Listed) {
	t.Helper()

	create := func(team client.Team, key string) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateLine(t.Context(), team.ID, client.Definition{
			"key": key, "name": "Delivery",
			"stations": []any{map[string]any{"key": "backlog", "name": "Backlog", "kind": "backlog"}},
		})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	create(seeded.OtherCore, "delivery")

	return create(seeded.Core, "delivery"), create(seeded.Platform, "review")
}

func TestLineListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	core, platform := seededLines(t, seeded)

	all := acctest.List(t, fake, "spotlight_line", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{core, platform}; !slices.Equal(all, want) {
		t.Errorf("listed %v, want %v", all, want)
	}

	inCore := acctest.List(t, fake, "spotlight_line", map[string]string{"workspace_id": seeded.Workspace.ID, "team_id": seeded.Core.ID})
	if want := []acctest.Listed{core}; !slices.Equal(inCore, want) {
		t.Errorf("listed %v in the core team, want %v", inCore, want)
	}
}

func TestLineListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	core, platform := seededLines(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query: true,
				Config: acctest.QueryConfig(
					acctest.ListBlock("spotlight_line", "all", map[string]string{"workspace_id": seeded.Workspace.ID}),
					acctest.ListBlock("spotlight_line", "core", map[string]string{"workspace_id": seeded.Workspace.ID, "team_id": seeded.Core.ID}),
				),
				QueryResultChecks: slices.Concat(
					acctest.ExpectListed("spotlight_line.all", core, platform),
					acctest.ExpectListed("spotlight_line.core", core),
				),
			},
		},
	})
}
