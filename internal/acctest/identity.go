package acctest

import (
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// ExpectIdentity checks that the resource at address is identified by its
// id, and by nothing else.
func ExpectIdentity(address string) []statecheck.StateCheck {
	return []statecheck.StateCheck{
		statecheck.ExpectIdentity(address, map[string]knownvalue.Check{"id": knownvalue.NotNull()}),
		statecheck.ExpectIdentityValueMatchesState(address, tfjsonpath.New("id")),
	}
}

// ImportedByIdentity is a test step that imports the resource at address
// with an import block naming the identity the prior step left it, and
// expects the import to plan no change.
func ImportedByIdentity(address string) resource.TestStep {
	return resource.TestStep{
		ResourceName:    address,
		ImportState:     true,
		ImportStateKind: resource.ImportBlockWithResourceIdentity,
	}
}
