package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededWorkspaceRunners(t *testing.T, seeded acctest.Seeded) acctest.Listed {
	t.Helper()

	declare := func(workspace client.Workspace) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateWorkspaceRunners(t.Context(), workspace.ID, client.WorkspaceRunnersInput{Trusts: []string{}, Pools: []string{"ephemeral"}})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	declare(seeded.Other)

	return declare(seeded.Workspace)
}

func TestWorkspaceRunnersListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	declared := seededWorkspaceRunners(t, seeded)

	listed := acctest.List(t, fake, "spotlight_workspace_runners", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{declared}; !slices.Equal(listed, want) {
		t.Errorf("listed %v, want %v", listed, want)
	}
}

func TestWorkspaceRunnersListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	declared := seededWorkspaceRunners(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query:             true,
				Config:            acctest.QueryConfig(acctest.ListBlock("spotlight_workspace_runners", "all", map[string]string{"workspace_id": seeded.Workspace.ID})),
				QueryResultChecks: acctest.ExpectListed("spotlight_workspace_runners.all", declared),
			},
		},
	})
}
