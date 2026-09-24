package provider_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func TestTeamListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)

	listed := acctest.List(t, fake, "spotlight_team", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{acctest.ListedAs(seeded.Core.Meta), acctest.ListedAs(seeded.Platform.Meta)}; !slices.Equal(listed, want) {
		t.Errorf("listed %v, want %v", listed, want)
	}
}

func TestTeamListResourceNeedsAWorkspaceItMaySee(t *testing.T) {
	fake := fakeapi.New(t)

	said := acctest.ListFails(t, fake, "spotlight_team", map[string]string{"workspace_id": "nowhere"})
	if !strings.Contains(said, "Could not list the teams") || !strings.Contains(said, "There is no workspace here that you may see") {
		t.Errorf("listing in a workspace nobody may see said:\n%s", said)
	}
}

func TestTeamListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query:             true,
				Config:            acctest.QueryConfig(acctest.ListBlock("spotlight_team", "all", map[string]string{"workspace_id": seeded.Workspace.ID})),
				QueryResultChecks: acctest.ExpectListed("spotlight_team.all", acctest.ListedAs(seeded.Core.Meta), acctest.ListedAs(seeded.Platform.Meta)),
			},
		},
	})
}
