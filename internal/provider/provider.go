// Package provider is the Spotlight Terraform provider: its configuration,
// and the resources, data sources and list resources it serves.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/expressions"
)

// Environment variables the provider reads its configuration from.
const (
	URLEnv   = "SPOTLIGHT_URL"
	TokenEnv = "SPOTLIGHT_API_TOKEN"
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
}

// Option adjusts how the provider reaches the control plane.
type Option func(*client.Config)

// New returns a factory for the provider at a version.
func New(version string, options ...Option) func() provider.Provider {
	return func() provider.Provider {
		return &SpotlightProvider{version: version, options: options}
	}
}

// Metadata names the provider spotlight.
func (p *SpotlightProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "spotlight"
	resp.Version = p.version
}

// Schema answers the provider block's schema.
func (p *SpotlightProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Configure a Spotlight workspace as code: its teams, lines, skills, integrations and harness profiles, its secrets, and its runners. " +
			"One API token granted the `configure`, `secrets` and `runners` scopes manages all of it.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The control plane's URL, as in `https://spotlight.example.com`. Defaults to `" + URLEnv + "`, which the `spotlight` CLI reads too.",
			},
			"token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "A Spotlight API token. Defaults to `" + TokenEnv + "`.",
			},
		},
	}
}

// Configure connects to the control plane the configuration names.
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

	for _, option := range p.options {
		option(&cfg)
	}

	if cfg.Token == "" {
		resp.Diagnostics.AddAttributeError(path.Root("token"), "No API token", "Set token, or "+TokenEnv+", to a Spotlight API token.")

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
		NewSecretResource,
		NewRunnerResource,
		NewRunnerTrustResource,
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

// ListResources lists the list resources the provider serves, which
// terraform query reads.
func (p *SpotlightProvider) ListResources(_ context.Context) []func() list.ListResource {
	return []func() list.ListResource{
		NewWorkspaceListResource,
		NewTeamListResource,
		NewLineListResource,
		NewSkillListResource,
		NewIntegrationListResource,
		NewHarnessProfileListResource,
		NewSecretListResource,
		NewRunnerListResource,
		NewRunnerTrustListResource,
		NewWorkspaceRunnersListResource,
	}
}

func orEnv(held types.String, env string) string {
	if !held.IsNull() && held.ValueString() != "" {
		return held.ValueString()
	}

	return os.Getenv(env)
}
