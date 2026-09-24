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

const workspaceDataSourceAddress = "data.spotlight_workspace.acme"

func TestWorkspaceDataSourceReadsTheWorkspaceBySlug(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + `
data "spotlight_workspace" "acme" {
  slug = spotlight_workspace.acme.slug
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs(workspaceDataSourceAddress, tfjsonpath.New("id"), "spotlight_workspace.acme", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue(workspaceDataSourceAddress, tfjsonpath.New("address"), knownvalue.StringExact("acme")),
					statecheck.ExpectKnownValue(workspaceDataSourceAddress, tfjsonpath.New("name"), knownvalue.StringExact("Acme")),
				},
			},
		},
	})
}

func TestWorkspaceDataSourceSaysNoWorkspaceHasAnUnknownSlug(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + `
data "spotlight_workspace" "nowhere" {
  slug = "nowhere"

  depends_on = [spotlight_workspace.acme]
}
`,
				ExpectError: regexp.MustCompile(`(?s)Could not read the workspace nowhere.*There is no workspace here that you may see`),
			},
		},
	})
}
