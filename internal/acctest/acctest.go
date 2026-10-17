// Package acctest serves the provider to the Terraform test harness, against
// a fake control plane or a real control plane. It also contains the checks and
// the configuration that the tests of the provider share.
package acctest

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
)

// Version is the provider version the tests serve.
const Version = "test"

// WriteOnly skips a test on a Terraform version that is too old for
// write-only attributes.
var WriteOnly = []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)}

// Identity skips a test on a Terraform version that is too old to import by
// resource identity.
var Identity = []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)}

// Query skips a test on a Terraform version that is too old for terraform
// query.
var Query = []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_14_0)}

// FakeProviderFactories serves the provider configured against fake.
func FakeProviderFactories(fake *fakeapi.Server) map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"spotlight": providerserver.NewProtocol6WithError(provider.New(Version, func(cfg *client.Config) {
			cfg.URL = fake.URL
			cfg.Token = fakeapi.Token
			cfg.Backoff = 0
		})()),
	}
}

// NoneLeft checks that the fake stores no resource of these kinds after
// Terraform destroyed all resources.
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

// NothingContains checks that no attribute in the state contains one of these
// values, because the state must never keep a write-only value.
func NothingContains(values ...func() string) func(*terraform.State) error {
	return func(state *terraform.State) error {
		for _, module := range state.Modules {
			for address, stored := range module.Resources {
				for name, kept := range stored.Primary.Attributes {
					for _, value := range values {
						if secret := value(); secret != "" && strings.Contains(kept, secret) {
							return fmt.Errorf("%s.%s contains a write-only value", address, name)
						}
					}
				}
			}
		}

		return nil
	}
}

// Value is a value known before the test runs.
func Value(value string) func() string {
	return func() string { return value }
}

// Workspace is configuration for a workspace and a team in it. Tests of the
// resources that a team has use it.
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
