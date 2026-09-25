package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededWorkspaceTokenTrusts(t *testing.T, seeded acctest.Seeded) (first, second acctest.Listed) {
	t.Helper()

	held := func(workspace client.Workspace, key string) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateWorkspaceTokenTrust(t.Context(), workspace.ID, client.WorkspaceTokenTrustInput{
			Key: key, Issuer: "https://token.actions.githubusercontent.com", Audience: "spotlight", Subject: "repo:acme/platform:*",
			Claims: map[string]string{}, Scopes: []string{"plan"}, LifetimeMinutes: 15,
		})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	held(seeded.Other, "github-main")
	later := held(seeded.Workspace, "github-pull-requests")

	return held(seeded.Workspace, "github-main"), later
}

func TestWorkspaceTokenTrustListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededWorkspaceTokenTrusts(t, seeded)

	listed := acctest.List(t, fake, "spotlight_workspace_token_trust", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{first, second}; !slices.Equal(listed, want) {
		t.Errorf("listed %v, want %v", listed, want)
	}
}

func TestWorkspaceTokenTrustListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededWorkspaceTokenTrusts(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query:             true,
				Config:            acctest.QueryConfig(acctest.ListBlock("spotlight_workspace_token_trust", "all", map[string]string{"workspace_id": seeded.Workspace.ID})),
				QueryResultChecks: acctest.ExpectListed("spotlight_workspace_token_trust.all", first, second),
			},
		},
	})
}
