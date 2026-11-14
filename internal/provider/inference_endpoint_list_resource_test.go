package provider_test

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

func seededInferenceEndpoint(t *testing.T, seeded acctest.Seeded) acctest.Listed {
	t.Helper()

	configure := func(workspace client.Workspace) acctest.Listed {
		t.Helper()

		created, err := seeded.Client.CreateInferenceEndpoint(t.Context(), workspace.ID, client.InferenceEndpointInput{BaseURL: "https://api.example.com/v1", Model: "gpt-5"})
		acctest.Must(t, err)

		return acctest.ListedAs(created.Meta)
	}

	configure(seeded.Other)

	return configure(seeded.Workspace)
}

func TestInferenceEndpointListResource(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	configured := seededInferenceEndpoint(t, seeded)

	listed := acctest.List(t, fake, "spotlight_inference_endpoint", map[string]string{"workspace_id": seeded.Workspace.ID})
	if want := []acctest.Listed{configured}; !slices.Equal(listed, want) {
		t.Errorf("listed %v, want %v", listed, want)
	}
}

func TestInferenceEndpointListResourceQuery(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	configured := seededInferenceEndpoint(t, seeded)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Query,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{Config: acctest.SeededConfig},
			{
				Query:             true,
				Config:            acctest.QueryConfig(acctest.ListBlock("spotlight_inference_endpoint", "all", map[string]string{"workspace_id": seeded.Workspace.ID})),
				QueryResultChecks: acctest.ExpectListed("spotlight_inference_endpoint.all", configured),
			},
		},
	})
}
