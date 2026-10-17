package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededEgresses(t *testing.T, seeded acctest.Seeded) (first, second acctest.Listed) {
	t.Helper()

	held := func(workspace client.Workspace, key, token string) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateEgress(t.Context(), workspace.ID, client.EgressInput{Key: key, URL: "https://" + key + ".example.com", Token: token})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	held(seeded.Other, "shared", tokenFirst)
	later := held(seeded.Workspace, "shared", tokenSecond)

	return held(seeded.Workspace, "regional", tokenThird), later
}

func TestEgressListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededEgresses(t, seeded)

	listed := acctest.List(t, fake, "spotlight_egress", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{first, second}; !slices.Equal(listed, want) {
		t.Errorf("listed %v, want %v", listed, want)
	}
}

func TestEgressListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededEgresses(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query:             true,
				Config:            acctest.QueryConfig(acctest.ListBlock("spotlight_egress", "all", map[string]string{"workspace_id": seeded.Workspace.ID})),
				QueryResultChecks: acctest.ExpectListed("spotlight_egress.all", first, second),
			},
		},
	})
}
