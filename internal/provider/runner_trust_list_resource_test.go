package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededRunnerTrusts(t *testing.T, seeded acctest.Seeded) (first, second acctest.Listed) {
	t.Helper()

	create := func(workspace client.Workspace, key string) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateRunnerTrust(t.Context(), workspace.ID, client.RunnerTrustInput{
			Key: key, Issuer: "https://oidc.example.com", Subject: "system:serviceaccount:spotlight:runner", Audience: "spotlight", Labels: []string{},
		})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	create(seeded.Other, "production-cluster")
	later := create(seeded.Workspace, "staging-cluster")

	return create(seeded.Workspace, "production-cluster"), later
}

func TestRunnerTrustListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededRunnerTrusts(t, seeded)

	listed := acctest.List(t, fake, "spotlight_runner_trust", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{first, second}; !slices.Equal(listed, want) {
		t.Errorf("listed %v, want %v", listed, want)
	}
}

func TestRunnerTrustListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededRunnerTrusts(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query:             true,
				Config:            acctest.QueryConfig(acctest.ListBlock("spotlight_runner_trust", "all", map[string]string{"workspace_id": seeded.Workspace.ID})),
				QueryResultChecks: acctest.ExpectListed("spotlight_runner_trust.all", first, second),
			},
		},
	})
}
