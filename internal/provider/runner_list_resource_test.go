package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededRunners(t *testing.T, seeded acctest.Seeded) (first, second acctest.Listed) {
	t.Helper()

	held := func(workspace client.Workspace, name string) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateRunner(t.Context(), workspace.ID, client.RunnerInput{Name: name, Labels: []string{}})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	held(seeded.Other, "build-1")
	later := held(seeded.Workspace, "build-2")

	return held(seeded.Workspace, "build-1"), later
}

func TestRunnerListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededRunners(t, seeded)

	listed := acctest.List(t, fake, "spotlight_runner", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{first, second}; !slices.Equal(listed, want) {
		t.Errorf("listed %v, want %v", listed, want)
	}
}

func TestRunnerListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededRunners(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query:             true,
				Config:            acctest.QueryConfig(acctest.ListBlock("spotlight_runner", "all", map[string]string{"workspace_id": seeded.Workspace.ID})),
				QueryResultChecks: acctest.ExpectListed("spotlight_runner.all", first, second),
			},
		},
	})
}
