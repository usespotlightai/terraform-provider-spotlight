package provider_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
)

func factories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"spotlight": providerserver.NewProtocol6WithError(provider.New("9.9.9")()),
	}
}

const workspace = `
resource "spotlight_workspace" "acme" {
  name = "Acme"
  slug = "acme"
}
`

func TestTheEnvironmentConfiguresTheProvider(t *testing.T) {
	fake := fakeapi.New(t)
	t.Setenv(provider.EndpointEnv, fake.URL)
	t.Setenv(provider.TokenEnv, fakeapi.Token)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: workspace,
				Check: func(*terraform.State) error {
					for _, each := range fake.Requests() {
						if got := each.Headers.Get("Spotlight-Client"); got != "terraform-provider/9.9.9" {
							return fmt.Errorf("%s %s sent Spotlight-Client %q", each.Method, each.Path, got)
						}
					}

					return nil
				},
			},
		},
	})
}

func TestTheProviderBlockConfiguresTheProvider(t *testing.T) {
	fake := fakeapi.New(t)
	t.Setenv(provider.EndpointEnv, "")
	t.Setenv(provider.TokenEnv, "")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "spotlight" {
  endpoint = %q
  token    = %q
}
`, fake.URL, fakeapi.Token) + workspace,
				Check: resource.TestCheckResourceAttr("spotlight_workspace.acme", "slug", "acme"),
			},
		},
	})
}

func TestTheProviderRefusesATokenThatIsNotSpotlights(t *testing.T) {
	fake := fakeapi.New(t)
	t.Setenv(provider.EndpointEnv, fake.URL)
	t.Setenv(provider.TokenEnv, "ghp_not_ours")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      workspace,
				ExpectError: regexp.MustCompile(`not a Spotlight API token`),
			},
		},
	})
}

func TestTheProviderNeedsAToken(t *testing.T) {
	fake := fakeapi.New(t)
	t.Setenv(provider.EndpointEnv, fake.URL)
	t.Setenv(provider.TokenEnv, "")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      workspace,
				ExpectError: regexp.MustCompile(`No API token`),
			},
		},
	})
}

func TestValidateNeedsNoControlPlane(t *testing.T) {
	t.Setenv(provider.EndpointEnv, "")
	t.Setenv(provider.TokenEnv, "")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "spotlight_integration" "linear" {
  team_id    = "team"
  definition = { integration = "linear", webhook = { events = [{ when = "$${{ body.tpye }}" }] } }
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`No API token`),
			},
		},
	})
}
