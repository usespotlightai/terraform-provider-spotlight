package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededOidcTrusts(t *testing.T, seeded acctest.Seeded) (first, second acctest.Listed) {
	t.Helper()

	create := func(workspace client.Workspace, key string) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateOidcTrust(t.Context(), workspace.ID, client.OidcTrustInput{
			Key: key, Issuer: "https://token.actions.githubusercontent.com", Audience: "spotlight", Subject: "repo:acme/platform:*",
			Claims: map[string]string{}, Scopes: []string{"plan"}, LifetimeMinutes: 15,
		})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	create(seeded.Other, "github-main")
	later := create(seeded.Workspace, "github-pull-requests")

	return create(seeded.Workspace, "github-main"), later
}

func TestOidcTrustListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededOidcTrusts(t, seeded)

	listed := acctest.List(t, fake, "spotlight_oidc_trust", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{first, second}; !slices.Equal(listed, want) {
		t.Errorf("listed %v, want %v", listed, want)
	}
}

func TestOidcTrustListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededOidcTrusts(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query:             true,
				Config:            acctest.QueryConfig(acctest.ListBlock("spotlight_oidc_trust", "all", map[string]string{"workspace_id": seeded.Workspace.ID})),
				QueryResultChecks: acctest.ExpectListed("spotlight_oidc_trust.all", first, second),
			},
		},
	})
}
