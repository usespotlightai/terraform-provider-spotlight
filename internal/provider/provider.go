// Package provider is the Spotlight Terraform provider. It contains the
// provider configuration and the resources, data sources and list resources
// that the provider serves.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/expressions"
)

// Environment variables that the provider reads its configuration from.
const (
	URLEnv          = "SPOTLIGHT_URL"
	TokenEnv        = "SPOTLIGHT_API_TOKEN"
	OIDCTrustEnv    = "SPOTLIGHT_OIDC_TRUST"
	OIDCAudienceEnv = "SPOTLIGHT_OIDC_AUDIENCE"
	OIDCTokenEnv    = "SPOTLIGHT_OIDC_TOKEN"
)

// Environment variables that GitHub Actions sets in a job with the
// id-token: write permission. The job uses them to request OIDC tokens.
const (
	GitHubTokenRequestURLEnv   = "ACTIONS_ID_TOKEN_REQUEST_URL"
	GitHubTokenRequestTokenEnv = "ACTIONS_ID_TOKEN_REQUEST_TOKEN"
)

var (
	_ provider.Provider                  = &SpotlightProvider{}
	_ provider.ProviderWithListResources = &SpotlightProvider{}
)

// SpotlightProvider is the Spotlight provider.
type SpotlightProvider struct {
	version string
	options []Option
}

// SpotlightProviderModel is the provider block.
type SpotlightProviderModel struct {
	URL   types.String `tfsdk:"url"`
	Token types.String `tfsdk:"token"`
	OIDC  *OIDCModel   `tfsdk:"oidc"`
}

// OIDCModel is the provider block's oidc block.
type OIDCModel struct {
	Trust    types.String `tfsdk:"trust"`
	Audience types.String `tfsdk:"audience"`
	Token    types.String `tfsdk:"token"`
}

// Option changes how the provider connects to the control plane.
type Option func(*client.Config)

// New returns a factory for the provider at a version.
func New(version string, options ...Option) func() provider.Provider {
	return func() provider.Provider {
		return &SpotlightProvider{version: version, options: options}
	}
}

// Metadata sets the name of the provider to spotlight.
func (p *SpotlightProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "spotlight"
	resp.Version = p.version
}

// Schema returns the schema of the provider block.
func (p *SpotlightProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Configure a Spotlight workspace as code: its teams, lines, skills, integrations and harness profiles, its secrets, and its runner pools and trusts. " +
			"On your machine, it authenticates as you after `spotlight login`, with only `url` set. " +
			"It also authenticates with a stored API token, or from CI with none: through `oidc`, it exchanges the OIDC token the CI platform issues the job for a short-lived token, through an OIDC trust. " +
			"One token granted the `configure`, `secrets`, `runners` and `trusts` scopes manages all of it.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The control plane's URL, as in `https://spotlight.example.com`. Defaults to `" + URLEnv + "`, which the `spotlight` CLI reads too.",
			},
			"token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "A Spotlight API token. Defaults to `" + TokenEnv + "`. Leave it unset to authenticate through `oidc`, or as the person who ran `spotlight login` for `url`.",
			},
		},
		Blocks: map[string]schema.Block{
			"oidc": schema.SingleNestedBlock{
				MarkdownDescription: "Authenticate from CI with no stored token: the OIDC token the CI platform issues the job is exchanged, through a `spotlight_oidc_trust`, for a short-lived token, renewed before it expires. " +
					"In GitHub Actions, a job granted `permissions: id-token: write` needs only `trust`; elsewhere, set the platform's OIDC token as `token`.",
				Attributes: map[string]schema.Attribute{
					"trust": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "The `id` of the OIDC trust to exchange through. Defaults to `" + OIDCTrustEnv + "`.",
					},
					"audience": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "The audience to request the OIDC token for, which must be the trust's. Defaults to `" + OIDCAudienceEnv + "`, then `" + client.DefaultAudience + "`.",
					},
					"token": schema.StringAttribute{
						Optional:  true,
						Sensitive: true,
						MarkdownDescription: "An OIDC token the CI platform issued the job, as GitLab's `id_tokens`, CircleCI's `$CIRCLE_OIDC_TOKEN_V2` or `buildkite-agent oidc request-token` return one. " +
							"Defaults to `" + OIDCTokenEnv + "`; in GitHub Actions, one is requested for each exchange.",
					},
				},
			},
		},
	}
}

// Configure connects to the control plane that the configuration identifies.
func (p *SpotlightProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config SpotlightProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.URL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("url"), "URL not known", "The url must be known when Terraform plans; set it from a variable or "+URLEnv+".")
	}

	if config.Token.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("token"), "Token not known", "The token must be known when Terraform plans; set it from a variable or "+TokenEnv+".")
	}

	oidc := config.OIDC
	if oidc == nil {
		oidc = &OIDCModel{Trust: types.StringNull(), Audience: types.StringNull(), Token: types.StringNull()}
	}

	for _, each := range []struct {
		name  string
		value types.String
	}{{"trust", oidc.Trust}, {"audience", oidc.Audience}, {"token", oidc.Token}} {
		if each.value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root("oidc").AtName(each.name), "OIDC "+each.name+" not known",
				"The oidc "+each.name+" must be known when Terraform plans; set it from a variable or the environment.")
		}
	}

	if resp.Diagnostics.HasError() {
		return
	}

	cfg := client.Config{
		URL:             orEnv(config.URL, URLEnv),
		Token:           orEnv(config.Token, TokenEnv),
		ProviderVersion: p.version,
		Retries:         3,
		Backoff:         client.DefaultBackoff,
	}

	if trust := orEnv(oidc.Trust, OIDCTrustEnv); trust != "" {
		if cfg.Token != "" {
			resp.Diagnostics.AddAttributeError(path.Root("oidc").AtName("trust"), "Both a token and an OIDC trust",
				"Authenticate with a stored API token (token or "+TokenEnv+") or through an OIDC trust (oidc.trust or "+OIDCTrustEnv+"), not both.")

			return
		}

		cfg.OIDC = &client.OIDC{
			Trust:    trust,
			Audience: orEnv(oidc.Audience, OIDCAudienceEnv),
			Workload: workloadTokens(oidc.Token, &resp.Diagnostics),
		}
	}

	for _, option := range p.options {
		option(&cfg)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	if cfg.Token == "" && cfg.OIDC == nil && cfg.URL != "" {
		login, found, err := client.StoredLogin(cfg.URL)
		if err != nil {
			resp.Diagnostics.AddError("Could not read the Spotlight login", err.Error())

			return
		}

		if found {
			cfg.Login = &login
		}
	}

	if cfg.Token == "" && cfg.OIDC == nil && cfg.Login == nil {
		resp.Diagnostics.AddAttributeError(path.Root("token"), "No API token",
			"The provider has no API token, OIDC trust or login for this Spotlight. "+
				"On your machine, run spotlight login. "+
				"Or set token, or "+TokenEnv+", to a Spotlight API token. "+
				"In CI, set oidc.trust, or "+OIDCTrustEnv+", to the id of an OIDC trust.")

		return
	}

	api, err := client.New(cfg)
	if err != nil {
		resp.Diagnostics.AddError("Could not configure the Spotlight provider", err.Error())

		return
	}

	data := &providerData{client: api, expressions: expressions.NewChecker(api)}
	resp.ResourceData = data
	resp.DataSourceData = data
	resp.ListResourceData = data
}

// Resources lists the resources the provider serves.
func (p *SpotlightProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewWorkspaceResource,
		NewTeamResource,
		NewLineResource,
		NewSkillResource,
		NewIntegrationResource,
		NewHarnessProfileResource,
		NewHookResource,
		NewSecretResource,
		NewOidcTrustResource,
		NewRunnerTrustResource,
		NewEgressResource,
		NewRunnerPoolResource,
		NewWorkspaceRunnersResource,
	}
}

// DataSources lists the data sources the provider serves.
func (p *SpotlightProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewWorkspaceDataSource,
		NewTeamDataSource,
	}
}

// ListResources lists the list resources that the provider serves.
// terraform query reads them.
func (p *SpotlightProvider) ListResources(_ context.Context) []func() list.ListResource {
	return []func() list.ListResource{
		NewWorkspaceListResource,
		NewTeamListResource,
		NewLineListResource,
		NewSkillListResource,
		NewIntegrationListResource,
		NewHarnessProfileListResource,
		NewHookListResource,
		NewSecretListResource,
		NewOidcTrustListResource,
		NewRunnerTrustListResource,
		NewEgressListResource,
		NewRunnerPoolListResource,
		NewWorkspaceRunnersListResource,
	}
}

func workloadTokens(configured types.String, diags *diag.Diagnostics) client.WorkloadTokens {
	if token := orEnv(configured, OIDCTokenEnv); token != "" {
		return client.SuppliedWorkloadToken(token)
	}

	requestURL, requestToken := os.Getenv(GitHubTokenRequestURLEnv), os.Getenv(GitHubTokenRequestTokenEnv)
	if requestURL != "" && requestToken != "" {
		return client.GitHubActions{RequestURL: requestURL, RequestToken: requestToken}
	}

	diags.AddAttributeError(path.Root("oidc").AtName("token"), "No OIDC token",
		"Exchanging through an OIDC trust needs an OIDC token the CI platform issued the job. "+
			"In GitHub Actions, grant the job permissions: id-token: write, which sets "+GitHubTokenRequestURLEnv+" and "+GitHubTokenRequestTokenEnv+" for the provider to request one. "+
			"Elsewhere, set oidc.token, or "+OIDCTokenEnv+", to the token the platform issues.")

	return nil
}

func orEnv(value types.String, env string) string {
	if !value.IsNull() && value.ValueString() != "" {
		return value.ValueString()
	}

	return os.Getenv(env)
}
