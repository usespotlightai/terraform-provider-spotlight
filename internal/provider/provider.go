// Package provider is the Spotlight Terraform provider: its configuration
// and the resources it serves.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/expressions"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/resources/integration"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/resources/line"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/resources/skill"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/resources/team"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/resources/workspace"
)

// Environment variables the provider reads its configuration from.
const (
	EndpointEnv = "SPOTLIGHT_URL"
	TokenEnv    = "SPOTLIGHT_API_TOKEN"
)

var _ provider.Provider = (*Provider)(nil)

// Provider is the Spotlight provider.
type Provider struct {
	version string
	options []Option
}

// Option adjusts how the provider reaches the control plane.
type Option func(*client.Config)

// New returns a factory for the provider at a version.
func New(version string, options ...Option) func() provider.Provider {
	return func() provider.Provider {
		return &Provider{version: version, options: options}
	}
}

type config struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Token    types.String `tfsdk:"token"`
}

// Metadata names the provider spotlight.
func (p *Provider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "spotlight"
	resp.Version = p.version
}

// Schema answers the provider block's schema.
func (p *Provider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Configure a Spotlight workspace as code: its teams, lines, skills and integrations.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The control plane's URL, as in `https://spotlight.example.com`. Defaults to `" + EndpointEnv + "`.",
			},
			"token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "A Spotlight API token, starting `spt_`. Defaults to `" + TokenEnv + "`.",
			},
		},
	}
}

// Configure connects to the control plane the configuration names.
func (p *Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var held config
	resp.Diagnostics.Append(req.Config.Get(ctx, &held)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if held.Endpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Endpoint not known", "The endpoint must be known when Terraform plans; set it from a variable or "+EndpointEnv+".")
	}

	if held.Token.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("token"), "Token not known", "The token must be known when Terraform plans; set it from a variable or "+TokenEnv+".")
	}

	if resp.Diagnostics.HasError() {
		return
	}

	cfg := client.Config{
		Endpoint:        orEnv(held.Endpoint, EndpointEnv),
		Token:           orEnv(held.Token, TokenEnv),
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

	clients := &resourcekit.Clients{API: api, Expressions: expressions.NewChecker(api.Expressions)}
	resp.ResourceData = clients
	resp.DataSourceData = clients
}

// Resources lists the resources the provider serves.
func (p *Provider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		workspace.New,
		team.New,
		line.New,
		skill.New,
		integration.New,
	}
}

// DataSources lists the data sources the provider serves: none yet.
func (p *Provider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func orEnv(held types.String, env string) string {
	if !held.IsNull() && held.ValueString() != "" {
		return held.ValueString()
	}

	return os.Getenv(env)
}
