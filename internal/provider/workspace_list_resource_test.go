package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func TestWorkspaceListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)

	listed := acctest.List(t, fake, "spotlight_workspace", nil)
	if want := []acctest.Listed{acctest.ListedAs(seeded.Workspace.Meta), acctest.ListedAs(seeded.Other.Meta)}; !slices.Equal(listed, want) {
		t.Errorf("listed %v, want %v", listed, want)
	}
}

func TestWorkspaceListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query:             true,
				Config:            acctest.QueryConfig(acctest.ListBlock("spotlight_workspace", "all", nil)),
				QueryResultChecks: acctest.ExpectListed("spotlight_workspace.all", acctest.ListedAs(seeded.Workspace.Meta), acctest.ListedAs(seeded.Other.Meta)),
			},
		},
	})
}
