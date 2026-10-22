package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
)

func providerFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"spotlight": providerserver.NewProtocol6WithError(provider.New("9.9.9")()),
	}
}

const providerWorkspace = `
resource "spotlight_workspace" "acme" {
  name = "Acme"
  slug = "acme"
}
`

func TestTheEnvironmentConfiguresTheProvider(t *testing.T) {
	fake := fakeapi.New(t)
	t.Setenv(provider.URLEnv, fake.URL)
	t.Setenv(provider.TokenEnv, fakeapi.Token)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerWorkspace,
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
	t.Setenv(provider.URLEnv, "")
	t.Setenv(provider.TokenEnv, "")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "spotlight" {
  url   = %q
  token = %q
}
`, fake.URL, fakeapi.Token) + providerWorkspace,
				Check: resource.TestCheckResourceAttr("spotlight_workspace.acme", "slug", "acme"),
			},
		},
	})
}

func TestTheProviderReportsATokenTheControlPlaneDoesNotAccept(t *testing.T) {
	fake := fakeapi.New(t)
	t.Setenv(provider.URLEnv, fake.URL)
	t.Setenv(provider.TokenEnv, "not-a-token-it-knows")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config:      providerWorkspace,
				ExpectError: regexp.MustCompile(`Send a Spotlight API token as a bearer token`),
			},
		},
	})
}

func TestTheProviderNeedsAToken(t *testing.T) {
	fake := fakeapi.New(t)
	t.Setenv(provider.URLEnv, fake.URL)
	t.Setenv(provider.TokenEnv, "")
	withoutLogin(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config:      providerWorkspace,
				ExpectError: regexp.MustCompile(`(?s)No API token.*run\s+spotlight\s+login`),
			},
		},
	})
}

func TestValidateNeedsNoControlPlane(t *testing.T) {
	t.Setenv(provider.URLEnv, "")
	t.Setenv(provider.TokenEnv, "")
	withoutLogin(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
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

func withoutCredentials(t *testing.T) {
	t.Helper()

	for _, env := range []string{
		provider.URLEnv, provider.TokenEnv, provider.OIDCTrustEnv, provider.OIDCAudienceEnv, provider.OIDCTokenEnv,
		provider.GitHubTokenRequestURLEnv, provider.GitHubTokenRequestTokenEnv,
	} {
		t.Setenv(env, "")
	}

	withoutLogin(t)
}

func withoutLogin(t *testing.T) {
	t.Helper()

	t.Setenv(client.ConfigDirEnv, t.TempDir())
}

func trustedGitHub(t *testing.T, fake *fakeapi.Server) string {
	t.Helper()

	trust, err := acctest.FakeClient(t, fake).CreateOidcTrust(t.Context(), mustWorkspace(t, fake), client.OidcTrustInput{
		Key: "github-main", Issuer: "https://token.actions.githubusercontent.com", Audience: "spotlight",
		Subject: "repo:acme/platform:ref:refs/heads/main", Claims: map[string]string{}, Scopes: []string{"configure"}, LifetimeMinutes: 15,
	})
	if err != nil {
		t.Fatal(err)
	}

	return trust.ID
}

func mustWorkspace(t *testing.T, fake *fakeapi.Server) string {
	t.Helper()

	workspace, err := acctest.FakeClient(t, fake).CreateWorkspace(t.Context(), client.WorkspaceInput{Name: "Globex", Slug: "globex"})
	if err != nil {
		t.Fatal(err)
	}

	return workspace.ID
}

type gitHubActions struct {
	mu       sync.Mutex
	requests []*http.Request
}

func (g *gitHubActions) serve(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.requests = append(g.requests, r.Clone(r.Context()))
		g.mu.Unlock()

		if r.Header.Get("Authorization") != "Bearer github-request-token" {
			http.Error(w, "bad request token", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": 1, "value": fakeapi.WorkloadToken})
	}))
	t.Cleanup(server.Close)

	return server
}

func everyCallUsedAnIssuedToken(fake *fakeapi.Server) resource.TestCheckFunc {
	return func(*terraform.State) error {
		exchanges := 0
		for _, each := range fake.Requests() {
			if each.Path == "/api/workspace-tokens/exchange" {
				exchanges++
				if got := each.Headers.Get("Spotlight-Client"); got != "terraform-provider/9.9.9" {
					return fmt.Errorf("exchanged with Spotlight-Client %q", got)
				}
				continue
			}

			if got := each.Headers.Get("Authorization"); exchanges > 0 && !strings.HasPrefix(got, "Bearer issued-") {
				return fmt.Errorf("%s %s sent Authorization %q, not an issued token", each.Method, each.Path, got)
			}
		}

		if exchanges == 0 {
			return fmt.Errorf("no token was exchanged")
		}

		return nil
	}
}

func TestTheProviderAuthenticatesInGitHubActionsThroughATrust(t *testing.T) {
	fake := fakeapi.New(t)
	withoutCredentials(t)

	github := &gitHubActions{}
	t.Setenv(provider.URLEnv, fake.URL)
	t.Setenv(provider.OIDCTrustEnv, trustedGitHub(t, fake))
	t.Setenv(provider.GitHubTokenRequestURLEnv, github.serve(t).URL+"/token?api-version=2.0")
	t.Setenv(provider.GitHubTokenRequestTokenEnv, "github-request-token")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerWorkspace,
				Check: resource.ComposeAggregateTestCheckFunc(
					everyCallUsedAnIssuedToken(fake),
					func(*terraform.State) error {
						github.mu.Lock()
						defer github.mu.Unlock()

						for _, each := range github.requests {
							if got := each.URL.Query().Get("audience"); got != "spotlight" {
								return fmt.Errorf("requested from GitHub the audience %q", got)
							}
						}

						return nil
					},
				),
			},
		},
	})
}

func TestTheOIDCBlockConfiguresTheProvider(t *testing.T) {
	fake := fakeapi.New(t)
	withoutCredentials(t)
	trust := trustedGitHub(t, fake)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "spotlight" {
  url = %q

  oidc {
    trust    = %q
    audience = "spotlight"
    token    = %q
  }
}
`, fake.URL, trust, fakeapi.WorkloadToken) + providerWorkspace,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_workspace.acme", "slug", "acme"),
					everyCallUsedAnIssuedToken(fake),
				),
			},
		},
	})
}

func TestTheProviderReportsAnOIDCTokenTheTrustDoesNotAccept(t *testing.T) {
	fake := fakeapi.New(t)
	withoutCredentials(t)
	t.Setenv(provider.URLEnv, fake.URL)
	t.Setenv(provider.OIDCTrustEnv, trustedGitHub(t, fake))
	t.Setenv(provider.OIDCTokenEnv, "a-token-no-trust-accepts")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config:      providerWorkspace,
				ExpectError: regexp.MustCompile(`(?s)could not exchange the OIDC\s+token`),
			},
		},
	})
}

func TestATrustWithNoOIDCTokenSaysWhatToSet(t *testing.T) {
	withoutCredentials(t)
	t.Setenv(provider.URLEnv, "https://spotlight.example.com")
	t.Setenv(provider.OIDCTrustEnv, "trust-1")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config:      providerWorkspace,
				ExpectError: regexp.MustCompile(`(?s)No OIDC token.*id-token: write.*SPOTLIGHT_OIDC_TOKEN`),
			},
		},
	})
}

func TestATokenAndATrustAreNotBothGiven(t *testing.T) {
	withoutCredentials(t)
	t.Setenv(provider.URLEnv, "https://spotlight.example.com")
	t.Setenv(provider.TokenEnv, "a-stored-token")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "spotlight" {
  oidc {
    trust = "trust-1"
    token = "a-workload-token"
  }
}
` + providerWorkspace,
				ExpectError: regexp.MustCompile(`Both a token and an OIDC trust`),
			},
		},
	})
}

func TestAnOIDCTrustMustBeKnownWhenPlanned(t *testing.T) {
	withoutCredentials(t)
	t.Setenv(provider.URLEnv, "https://spotlight.example.com")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "terraform_data" "trust" {
  input = "trust-1"
}

provider "spotlight" {
  oidc {
    trust = terraform_data.trust.output
  }
}
` + providerWorkspace,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`OIDC trust not known`),
			},
		},
	})
}

func signedIn(t *testing.T, address, token string) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv(client.ConfigDirEnv, dir)

	content := fmt.Sprintf(`{"logins": {%q: {"token": %q, "user": "person@example.com"}}}`, address, token)
	path := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestASpotlightLoginAuthenticatesTheProvider(t *testing.T) {
	fake := fakeapi.New(t)
	withoutCredentials(t)
	signedIn(t, fake.URL, fakeapi.Token)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "spotlight" {
  url = %q
}
`, strings.Replace(fake.URL, "http://", "HTTP://", 1)+"/api/") + providerWorkspace,
				Check: func(*terraform.State) error {
					for _, each := range fake.Requests() {
						if got := each.Headers.Get("Authorization"); got != "Bearer "+fakeapi.Token {
							return fmt.Errorf("%s %s sent Authorization %q, not the login's token", each.Method, each.Path, got)
						}
					}

					return nil
				},
			},
		},
	})
}

func TestAStoredTokenWinsOverALogin(t *testing.T) {
	fake := fakeapi.New(t)
	withoutCredentials(t)
	signedIn(t, fake.URL, "a-login-token-it-does-not-know")
	t.Setenv(provider.URLEnv, fake.URL)
	t.Setenv(provider.TokenEnv, fakeapi.Token)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerWorkspace,
				Check:  resource.TestCheckResourceAttr("spotlight_workspace.acme", "slug", "acme"),
			},
		},
	})
}

func TestATrustWinsOverALogin(t *testing.T) {
	fake := fakeapi.New(t)
	withoutCredentials(t)
	signedIn(t, fake.URL, fakeapi.Token)

	t.Setenv(provider.URLEnv, fake.URL)
	t.Setenv(provider.OIDCTrustEnv, trustedGitHub(t, fake))
	t.Setenv(provider.OIDCTokenEnv, fakeapi.WorkloadToken)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerWorkspace,
				Check:  everyCallUsedAnIssuedToken(fake),
			},
		},
	})
}

func TestARejectedLoginSaysToRunSpotlightLogin(t *testing.T) {
	fake := fakeapi.New(t)
	withoutCredentials(t)
	signedIn(t, fake.URL, "an-expired-session")
	t.Setenv(provider.URLEnv, fake.URL)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config:      providerWorkspace,
				ExpectError: regexp.MustCompile(`(?s)did\s+not\s+accept\s+the\s+login\s+of\s+person@example.com.*run\s+spotlight\s+login\s+to\s+sign\s+in\s+again`),
			},
		},
	})
}

func TestALoginForAnotherSpotlightIsNotUsed(t *testing.T) {
	fake := fakeapi.New(t)
	withoutCredentials(t)
	signedIn(t, "https://app.usespotlight.ai", fakeapi.Token)
	t.Setenv(provider.URLEnv, fake.URL)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config:      providerWorkspace,
				ExpectError: regexp.MustCompile(`(?s)No API token.*run\s+spotlight\s+login`),
			},
		},
	})
}

func TestACredentialsFileThatDoesNotParseIsNamed(t *testing.T) {
	fake := fakeapi.New(t)
	withoutCredentials(t)
	path := signedIn(t, fake.URL, fakeapi.Token)
	if err := os.WriteFile(path, []byte(`{"logins": `), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(provider.URLEnv, fake.URL)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config:      providerWorkspace,
				ExpectError: regexp.MustCompile(`(?s)Could not read the Spotlight login.*` + regexp.QuoteMeta(path)),
			},
		},
	})
}
