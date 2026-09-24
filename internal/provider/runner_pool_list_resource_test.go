package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededRunnerPools(t *testing.T, seeded acctest.Seeded) (first, second acctest.Listed) {
	t.Helper()

	held := func(workspace client.Workspace, key string, provision *client.PoolProvision) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateRunnerPool(t.Context(), workspace.ID, client.RunnerPoolInput{Key: key, Labels: []string{}, Provision: provision})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	held(seeded.Other, "ephemeral", nil)
	later := held(seeded.Workspace, "k8s-jobs", &client.PoolProvision{URL: "https://provision.example.com/runs", Secret: provisionFirst})

	return held(seeded.Workspace, "ephemeral", nil), later
}

func TestRunnerPoolListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededRunnerPools(t, seeded)

	listed := acctest.List(t, fake, "spotlight_runner_pool", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{first, second}; !slices.Equal(listed, want) {
		t.Errorf("listed %v, want %v", listed, want)
	}
}

func TestRunnerPoolListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededRunnerPools(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query:             true,
				Config:            acctest.QueryConfig(acctest.ListBlock("spotlight_runner_pool", "all", map[string]string{"workspace_id": seeded.Workspace.ID})),
				QueryResultChecks: acctest.ExpectListed("spotlight_runner_pool.all", first, second),
			},
		},
	})
}
