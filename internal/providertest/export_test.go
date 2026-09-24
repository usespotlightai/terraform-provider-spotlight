package providertest_test

import (
	"testing"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/providertest"
)

const adopted = `import {
  to = spotlight_workspace.acme
  id = "acme"
}

resource "spotlight_workspace" "acme" {
  name = "Acme"
  slug = "acme"
}

resource "spotlight_team" "core" {
  workspace_id = spotlight_workspace.acme.id
  key          = "core"
  name         = "Core"
}
`

func TestProtectedPreventsDestroyingOnlyTheResourceNamed(t *testing.T) {
	want := `import {
  to = spotlight_workspace.acme
  id = "acme"
}

resource "spotlight_workspace" "acme" {
  name = "Acme"
  slug = "acme"

  lifecycle {
    prevent_destroy = true
  }
}

resource "spotlight_team" "core" {
  workspace_id = spotlight_workspace.acme.id
  key          = "core"
  name         = "Core"
}
`

	if got := providertest.Protected(adopted, "spotlight_workspace.acme"); got != want {
		t.Errorf("got\n%s", got)
	}

	if got := providertest.Protected(adopted, "spotlight_workspace.other"); got != adopted {
		t.Errorf("protected what it does not hold:\n%s", got)
	}
}
