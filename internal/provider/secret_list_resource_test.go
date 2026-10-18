package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededSecrets(t *testing.T, seeded acctest.Seeded) (first, second acctest.Listed) {
	t.Helper()

	create := func(workspace client.Workspace, name string) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateSecret(t.Context(), workspace.ID, client.SecretInput{
			Name:     name,
			Source:   client.SecretSource{Kind: client.SourceAWSSecretsManager, SecretID: "acme/" + name},
			Bindings: []client.SecretBinding{},
		})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	create(seeded.Other, "GITHUB_TOKEN")
	later := create(seeded.Workspace, "LINEAR_API_KEY")

	return create(seeded.Workspace, "GITHUB_TOKEN"), later
}

func TestSecretListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededSecrets(t, seeded)

	listed := acctest.List(t, fake, "spotlight_secret", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{first, second}; !slices.Equal(listed, want) {
		t.Errorf("listed %v, want %v", listed, want)
	}
}

func TestSecretListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	first, second := seededSecrets(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query:             true,
				Config:            acctest.QueryConfig(acctest.ListBlock("spotlight_secret", "all", map[string]string{"workspace_id": seeded.Workspace.ID})),
				QueryResultChecks: acctest.ExpectListed("spotlight_secret.all", first, second),
			},
		},
	})
}
