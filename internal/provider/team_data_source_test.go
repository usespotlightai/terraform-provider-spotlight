package provider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

const teamDataSourceAddress = "data.spotlight_team.core"

func TestTeamDataSourceReadsTheTeamByKey(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + `
data "spotlight_team" "core" {
  workspace_id = spotlight_workspace.acme.id
  key          = spotlight_team.core.key
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs(teamDataSourceAddress, tfjsonpath.New("id"), "spotlight_team.core", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.CompareValuePairs(teamDataSourceAddress, tfjsonpath.New("workspace_id"), "spotlight_workspace.acme", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue(teamDataSourceAddress, tfjsonpath.New("address"), knownvalue.StringExact("core")),
					statecheck.ExpectKnownValue(teamDataSourceAddress, tfjsonpath.New("name"), knownvalue.StringExact("Core")),
				},
			},
		},
	})
}

func TestTeamDataSourceSaysNoTeamHasAnUnknownKey(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + `
data "spotlight_team" "nowhere" {
  workspace_id = spotlight_workspace.acme.id
  key          = "nowhere"

  depends_on = [spotlight_team.core]
}
`,
				ExpectError: regexp.MustCompile(`(?s)Could not read the team nowhere.*There is no team here that you may see`),
			},
		},
	})
}
