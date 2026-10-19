package acctest

import (
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// ExpectIdentity checks that the identity of the resource at address
// contains its id and nothing else.
func ExpectIdentity(address string) []statecheck.StateCheck {
	return []statecheck.StateCheck{
		statecheck.ExpectIdentity(address, map[string]knownvalue.Check{"id": knownvalue.NotNull()}),
		statecheck.ExpectIdentityValueMatchesState(address, tfjsonpath.New("id")),
	}
}

// ImportedByIdentity is a test step that imports the resource at address
// with an import block. The block contains the identity that the previous
// step set on the resource. The step expects a plan with no change.
func ImportedByIdentity(address string) resource.TestStep {
	return resource.TestStep{
		ResourceName:    address,
		ImportState:     true,
		ImportStateKind: resource.ImportBlockWithResourceIdentity,
	}
}
