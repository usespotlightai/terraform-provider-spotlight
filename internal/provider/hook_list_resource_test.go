package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededHooks(t *testing.T, seeded acctest.Seeded) (core, platform acctest.Listed) {
	t.Helper()

	held := func(team client.Team, key string) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateHook(t.Context(), team.ID, client.HookInput{
			Key:     key,
			On:      "run.ended",
			Steps:   []any{map[string]any{"name": "audit", "request": map[string]any{"url": "https://audit.acme.dev"}}},
			Signing: &client.HookSigning{OIDC: &client.HookOIDC{Audience: "https://audit.acme.dev"}},
		})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	held(seeded.OtherCore, "audit")

	return held(seeded.Core, "audit"), held(seeded.Platform, "audit")
}

func TestHookListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	core, platform := seededHooks(t, seeded)

	all := acctest.List(t, fake, "spotlight_hook", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{core, platform}; !slices.Equal(all, want) {
		t.Errorf("listed %v, want %v", all, want)
	}

	held := acctest.List(t, fake, "spotlight_hook", map[string]string{"workspace_id": seeded.Workspace.ID, "team_id": seeded.Core.ID})
	if want := []acctest.Listed{core}; !slices.Equal(held, want) {
		t.Errorf("listed %v in the core team, want %v", held, want)
	}
}

func TestHookListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	core, platform := seededHooks(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query: true,
				Config: acctest.QueryConfig(
					acctest.ListBlock("spotlight_hook", "all", map[string]string{"workspace_id": seeded.Workspace.ID}),
					acctest.ListBlock("spotlight_hook", "core", map[string]string{"workspace_id": seeded.Workspace.ID, "team_id": seeded.Core.ID}),
				),
				QueryResultChecks: slices.Concat(
					acctest.ExpectListed("spotlight_hook.all", core, platform),
					acctest.ExpectListed("spotlight_hook.core", core),
				),
			},
		},
	})
}
