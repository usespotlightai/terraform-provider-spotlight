package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededSkills(t *testing.T, seeded acctest.Seeded) (core, platform acctest.Listed) {
	t.Helper()

	create := func(team client.Team, key string) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateSkill(t.Context(), team.ID, client.SkillInput{Key: key, Files: map[string]string{
			"SKILL.md": "---\nname: " + key + "\ndescription: Look up dashboards\n---\n",
		}})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	create(seeded.OtherCore, "grafana")

	return create(seeded.Core, "grafana"), create(seeded.Platform, "sentry")
}

func TestSkillListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	core, platform := seededSkills(t, seeded)

	all := acctest.List(t, fake, "spotlight_skill", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{core, platform}; !slices.Equal(all, want) {
		t.Errorf("listed %v, want %v", all, want)
	}

	inCore := acctest.List(t, fake, "spotlight_skill", map[string]string{"workspace_id": seeded.Workspace.ID, "team_id": seeded.Core.ID})
	if want := []acctest.Listed{core}; !slices.Equal(inCore, want) {
		t.Errorf("listed %v in the core team, want %v", inCore, want)
	}
}

func TestSkillListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	core, platform := seededSkills(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query: true,
				Config: acctest.QueryConfig(
					acctest.ListBlock("spotlight_skill", "all", map[string]string{"workspace_id": seeded.Workspace.ID}),
					acctest.ListBlock("spotlight_skill", "core", map[string]string{"workspace_id": seeded.Workspace.ID, "team_id": seeded.Core.ID}),
				),
				QueryResultChecks: slices.Concat(
					acctest.ExpectListed("spotlight_skill.all", core, platform),
					acctest.ExpectListed("spotlight_skill.core", core),
				),
			},
		},
	})
}
