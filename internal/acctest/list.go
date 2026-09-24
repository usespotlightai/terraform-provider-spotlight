package acctest

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

// Listed is one resource a list resource found: the id its identity names
// and the name it is displayed by.
type Listed struct {
	ID          string
	DisplayName string
}

// ListedAs is how a list resource finds the resource meta describes.
func ListedAs(meta client.Meta) Listed {
	return Listed{ID: meta.ID, DisplayName: meta.Address}
}

// ExpectListed checks that the list block at address found exactly these
// resources.
func ExpectListed(address string, listed ...Listed) []querycheck.QueryResultCheck {
	checks := []querycheck.QueryResultCheck{querycheck.ExpectLength(address, len(listed))}

	for _, each := range listed {
		identity := map[string]knownvalue.Check{"id": knownvalue.StringExact(each.ID)}
		checks = append(checks,
			querycheck.ExpectIdentity(address, identity),
			querycheck.ExpectResourceDisplayName(address, queryfilter.ByResourceIdentity(identity), knownvalue.StringExact(each.DisplayName)),
		)
	}

	return checks
}

// QueryConfig is a .tfquery.hcl file of these list blocks, each served by
// the provider.
func QueryConfig(blocks ...string) string {
	return "provider \"spotlight\" {}\n" + strings.Join(blocks, "")
}

// ListBlock is a list block of the list resource kind, named name, with
// config as its config block.
func ListBlock(kind, name string, config map[string]string) string {
	var block strings.Builder
	fmt.Fprintf(&block, "\nlist %q %q {\n  provider = spotlight\n", kind, name)

	if len(config) > 0 {
		block.WriteString("\n  config {\n")
		for _, attribute := range slices.Sorted(maps.Keys(config)) {
			fmt.Fprintf(&block, "    %s = %q\n", attribute, config[attribute])
		}
		block.WriteString("  }\n")
	}

	block.WriteString("}\n")

	return block.String()
}

// List asks the provider, configured against fake, what the list resource
// kind finds with config, as terraform query would with include_resource,
// and fails the test if it reports an error or answers a resource that is
// not the one its identity names.
func List(t *testing.T, fake *fakeapi.Server, kind string, config map[string]string) []Listed {
	t.Helper()

	listed, problems := list(t, fake, kind, config)
	if len(problems) > 0 {
		t.Fatalf("listing %s failed:\n%s", kind, strings.Join(problems, "\n"))
	}

	return listed
}

// ListFails asks the provider what the list resource kind finds with
// config, and answers the errors it reports, failing the test if there are
// none.
func ListFails(t *testing.T, fake *fakeapi.Server, kind string, config map[string]string) string {
	t.Helper()

	_, problems := list(t, fake, kind, config)
	if len(problems) == 0 {
		t.Fatalf("listing %s did not fail", kind)
	}

	return strings.Join(problems, "\n")
}

var identityType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{"id": tftypes.String}}

func list(t *testing.T, fake *fakeapi.Server, kind string, config map[string]string) ([]Listed, []string) {
	t.Helper()

	ctx := t.Context()
	server, schemas := ConfiguredServer(t, fake)

	listSchema, ok := schemas.ListResourceSchemas[kind]
	if !ok {
		t.Fatalf("the provider serves no list resource %s", kind)
	}

	configType, ok := listSchema.ValueType().(tftypes.Object)
	if !ok {
		t.Fatalf("the list resource %s is configured by %s, not an object", kind, listSchema.ValueType())
	}

	values := map[string]tftypes.Value{}
	for name := range configType.AttributeTypes {
		values[name] = tftypes.NewValue(tftypes.String, nil)
		if given, ok := config[name]; ok {
			values[name] = tftypes.NewValue(tftypes.String, given)
		}
	}

	listConfig, err := tfprotov6.NewDynamicValue(configType, tftypes.NewValue(configType, values))
	if err != nil {
		t.Fatal(err)
	}

	lister, ok := server.(tfprotov6.ListResourceServer)
	if !ok {
		t.Fatal("the provider server lists nothing")
	}

	stream, err := lister.ListResource(ctx, &tfprotov6.ListResourceRequest{TypeName: kind, Config: &listConfig, IncludeResource: true})
	if err != nil {
		t.Fatal(err)
	}

	resourceType := schemas.ResourceSchemas[kind].ValueType()

	var listed []Listed
	var problems []string
	for result := range stream.Results {
		for _, each := range result.Diagnostics {
			if each.Severity == tfprotov6.DiagnosticSeverityError {
				problems = append(problems, each.Summary+": "+each.Detail)
			}
		}
		if result.Identity == nil {
			continue
		}

		id := stringAt(t, result.Identity.IdentityData, identityType, "id")
		if result.Resource == nil {
			problems = append(problems, fmt.Sprintf("%s answered no resource", result.DisplayName))
		} else if held := stringAt(t, result.Resource, resourceType, "id"); held != id {
			problems = append(problems, fmt.Sprintf("%s answered the resource %s, but its identity names %s", result.DisplayName, held, id))
		}

		listed = append(listed, Listed{ID: id, DisplayName: result.DisplayName})
	}

	return listed, problems
}

// ConfiguredServer serves the provider configured against fake, as
// Terraform would after configuring it, and answers its schemas.
func ConfiguredServer(t *testing.T, fake *fakeapi.Server) (tfprotov6.ProviderServer, *tfprotov6.GetProviderSchemaResponse) {
	t.Helper()

	ctx := t.Context()

	server, err := FakeProviderFactories(fake)["spotlight"]()
	if err != nil {
		t.Fatal(err)
	}

	schemas, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}

	providerConfig, err := tfprotov6.NewDynamicValue(schemas.Provider.ValueType(), tftypes.NewValue(schemas.Provider.ValueType(), map[string]tftypes.Value{
		"url":   tftypes.NewValue(tftypes.String, nil),
		"token": tftypes.NewValue(tftypes.String, nil),
	}))
	if err != nil {
		t.Fatal(err)
	}

	configured, err := server.ConfigureProvider(ctx, &tfprotov6.ConfigureProviderRequest{Config: &providerConfig})
	if err != nil {
		t.Fatal(err)
	}

	for _, each := range configured.Diagnostics {
		if each.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("configuring the provider failed: %s: %s", each.Summary, each.Detail)
		}
	}

	return server, schemas
}

func stringAt(t *testing.T, held *tfprotov6.DynamicValue, kind tftypes.Type, name string) string {
	t.Helper()

	value, err := held.Unmarshal(kind)
	if err != nil {
		t.Fatal(err)
	}

	var attributes map[string]tftypes.Value
	if err := value.As(&attributes); err != nil {
		t.Fatal(err)
	}

	var out string
	if err := attributes[name].As(&out); err != nil {
		t.Fatal(err)
	}

	return out
}
