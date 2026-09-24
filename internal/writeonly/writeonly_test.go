package writeonly_test

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/writeonly"
)

type sent struct {
	mu     sync.Mutex
	values []string
}

func (s *sent) add(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = append(s.values, value)
}

func (s *sent) all() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.values)
}

type secretProvider struct{ sent *sent }

func (p secretProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "spotlight"
}

func (p secretProvider) Schema(context.Context, provider.SchemaRequest, *provider.SchemaResponse) {}

func (p secretProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}

func (p secretProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{func() resource.Resource { return secret(p) }}
}

func (p secretProvider) DataSources(context.Context) []func() datasource.DataSource { return nil }

type secret struct{ sent *sent }

type secretModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Value        types.String `tfsdk:"value_wo"`
	ValueVersion types.Int64  `tfsdk:"value_wo_version"`
}

func (s secret) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test_secret"
}

func (s secret) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := writeonly.Attributes("secret's value")
	attributes["id"] = schema.StringAttribute{
		Computed:      true,
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
	attributes["name"] = schema.StringAttribute{Required: true}
	resp.Schema = schema.Schema{Attributes: attributes}
}

func (s secret) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan secretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	value, diags := writeonly.Value(ctx, req.Config)
	resp.Diagnostics.Append(diags...)
	s.sent.add(value)

	plan.ID = types.StringValue(plan.Name.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (s secret) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state secretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (s secret) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan secretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	rewritten, diags := writeonly.Rewritten(ctx, req.Plan, req.State)
	resp.Diagnostics.Append(diags...)

	if rewritten {
		value, diags := writeonly.Value(ctx, req.Config)
		resp.Diagnostics.Append(diags...)
		s.sent.add(value)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (s secret) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {}

func secretConfig(value string, version int) string {
	return fmt.Sprintf(`
resource "spotlight_test_secret" "token" {
  name             = "GRAFANA_TOKEN"
  value_wo         = %q
  value_wo_version = %d
}
`, value, version)
}

func expectSent(sent *sent, want ...string) error {
	if got := sent.all(); !slices.Equal(got, want) {
		return fmt.Errorf("sent %q, want %q", got, want)
	}

	return nil
}

func TestWriteOnlyValueIsSentButNeverStored(t *testing.T) {
	sent := &sent{}

	tfresource.UnitTest(t, tfresource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"spotlight": providerserver.NewProtocol6WithError(secretProvider{sent: sent}),
		},
		Steps: []tfresource.TestStep{
			{
				Config: secretConfig("first", 1),
				Check: tfresource.ComposeTestCheckFunc(
					tfresource.TestCheckNoResourceAttr("spotlight_test_secret.token", writeonly.ValueAttribute),
					tfresource.TestCheckResourceAttr("spotlight_test_secret.token", writeonly.VersionAttribute, "1"),
					func(*terraform.State) error { return expectSent(sent, "first") },
				),
			},
			{
				Config:   secretConfig("second", 1),
				PlanOnly: true,
			},
			{
				Config: secretConfig("second", 2),
				Check: tfresource.ComposeTestCheckFunc(
					tfresource.TestCheckNoResourceAttr("spotlight_test_secret.token", writeonly.ValueAttribute),
					func(*terraform.State) error { return expectSent(sent, "first", "second") },
				),
			},
		},
	})
}

func TestWriteOnlyValueIsRequired(t *testing.T) {
	tfresource.UnitTest(t, tfresource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"spotlight": providerserver.NewProtocol6WithError(secretProvider{sent: &sent{}}),
		},
		Steps: []tfresource.TestStep{
			{
				Config:      `resource "spotlight_test_secret" "token" { name = "GRAFANA_TOKEN" }`,
				ExpectError: regexp.MustCompile(`value_wo`),
			},
		},
	})
}
