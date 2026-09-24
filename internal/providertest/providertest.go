// Package providertest serves the provider to Terraform's test harness,
// pointed at a fake control plane.
package providertest

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
)

// Version is the provider version the tests serve.
const Version = "test"

// Factories serves the provider configured against fake.
func Factories(fake *fakeapi.Server) map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"spotlight": providerserver.NewProtocol6WithError(provider.New(Version, func(cfg *client.Config) {
			cfg.Endpoint = fake.URL
			cfg.Token = fakeapi.Token
			cfg.Backoff = 0
		})()),
	}
}

// NoneLeft checks the fake holds nothing of these kinds once Terraform has
// destroyed everything.
func NoneLeft(fake *fakeapi.Server, kinds ...string) func(*terraform.State) error {
	return func(*terraform.State) error {
		for _, kind := range kinds {
			if left := fake.Count(kind); left > 0 {
				return fmt.Errorf("%d %s left after destroy", left, kind)
			}
		}

		return nil
	}
}

// Workspace is configuration for a workspace and a team in it, for the
// resources a team holds.
const Workspace = `
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
