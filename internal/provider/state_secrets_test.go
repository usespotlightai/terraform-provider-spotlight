package provider_test

import (
	"context"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
)

func TestNoResourceOrDataSourceStoresASecretInTheState(t *testing.T) {
	server, err := providerserver.NewProtocol6WithError(provider.New("9.9.9")())()
	if err != nil {
		t.Fatal(err)
	}

	resp, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}

	for _, each := range resp.Diagnostics {
		if each.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("%s: %s", each.Summary, each.Detail)
		}
	}

	if len(resp.ResourceSchemas) == 0 || len(resp.DataSourceSchemas) == 0 {
		t.Fatal("the provider has no resource or data source schemas")
	}

	var stored []string
	for _, schemas := range []map[string]*tfprotov6.Schema{resp.ResourceSchemas, resp.DataSourceSchemas} {
		for name, schema := range schemas {
			stored = append(stored, secretsInState(name, schema.Block)...)
		}
	}

	slices.Sort(stored)
	for _, each := range stored {
		t.Errorf("%s is sensitive but not write-only, so the state stores it", each)
	}
}

func TestSecretsInStateFindsASensitiveAttributeThatIsNotWriteOnly(t *testing.T) {
	block := &tfprotov6.SchemaBlock{
		Attributes: []*tfprotov6.SchemaAttribute{
			{Name: "value_wo", Sensitive: true, WriteOnly: true},
			{Name: "plain"},
			{Name: "signing", NestedType: &tfprotov6.SchemaObject{
				Attributes: []*tfprotov6.SchemaAttribute{
					{Name: "secret", Sensitive: true},
				},
			}},
		},
		BlockTypes: []*tfprotov6.SchemaNestedBlock{
			{TypeName: "provision", Block: &tfprotov6.SchemaBlock{
				Attributes: []*tfprotov6.SchemaAttribute{
					{Name: "token", Sensitive: true},
				},
			}},
		},
	}

	got := secretsInState("spotlight_example", block)
	want := []string{"spotlight_example.signing.secret", "spotlight_example.provision.token"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func secretsInState(prefix string, block *tfprotov6.SchemaBlock) []string {
	if block == nil {
		return nil
	}

	var stored []string
	for _, each := range block.Attributes {
		stored = append(stored, attributeSecretsInState(prefix+"."+each.Name, each)...)
	}

	for _, each := range block.BlockTypes {
		stored = append(stored, secretsInState(prefix+"."+each.TypeName, each.Block)...)
	}

	return stored
}

func attributeSecretsInState(at string, attribute *tfprotov6.SchemaAttribute) []string {
	var stored []string
	if attribute.Sensitive && !attribute.WriteOnly {
		stored = append(stored, at)
	}

	if attribute.NestedType != nil {
		for _, each := range attribute.NestedType.Attributes {
			stored = append(stored, attributeSecretsInState(at+"."+each.Name, each)...)
		}
	}

	return stored
}
