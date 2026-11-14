package provider_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

const inferenceEndpointAddress = "spotlight_inference_endpoint.acme"

const (
	inferenceKeyFirst  = "sk-first-0123456789abcdef"
	inferenceKeySecond = "sk-second-0123456789abcdef"
)

func inferenceEndpointAt(baseURL, model string, reasoning bool, key string, version int) string {
	return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_inference_endpoint" "acme" {
  workspace_id              = spotlight_workspace.acme.id
  base_url                  = %q
  model                     = %q
  supports_reasoning_effort = %t
  api_key_wo                = %q
  api_key_wo_version        = %d
}
`, baseURL, model, reasoning, key, version)
}

func inferenceEndpointKeepingKey(baseURL, model string, version int) string {
	return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_inference_endpoint" "acme" {
  workspace_id       = spotlight_workspace.acme.id
  base_url           = %q
  model              = %q
  api_key_wo_version = %d
}
`, baseURL, model, version)
}

func inferenceEndpointKeyless(baseURL, model string) string {
	return acctest.Workspace + fmt.Sprintf(`
resource "spotlight_inference_endpoint" "acme" {
  workspace_id = spotlight_workspace.acme.id
  base_url     = %q
  model        = %q
}
`, baseURL, model)
}

func inferenceEndpointImportID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/acme", nil
}

func inferenceEndpointStoredKey(fake *fakeapi.Server) string {
	return fake.WriteOnly(fake.IDOf("inference-endpoint", "acme"))
}

func inferenceEndpointStores(fake *fakeapi.Server, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := inferenceEndpointStoredKey(fake); got != want {
			return fmt.Errorf("the inference endpoint stores %q, want %q", got, want)
		}

		return nil
	}
}

func inferenceEndpointKey(body map[string]any) (any, bool) {
	if _, ok := body["baseUrl"]; !ok {
		return nil, false
	}

	key, ok := body["apiKeyWo"]

	return key, ok
}

func inferenceEndpointAnswer(fake *fakeapi.Server) resource.TestCheckFunc {
	return func(*terraform.State) error {
		req, err := http.NewRequest(http.MethodGet, fake.URL+"/api/v1/resources/inference-endpoints/"+fake.IDOf("inference-endpoint", "acme"), nil)
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "Bearer "+fakeapi.Token)
		req.Header.Set("Spotlight-Client", "terraform-provider/test")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		for _, key := range []string{inferenceKeyFirst, inferenceKeySecond} {
			if strings.Contains(string(raw), key) {
				return fmt.Errorf("the API returned an API key: %s", raw)
			}
		}

		if strings.Contains(string(raw), "apiKeyWo\"") {
			return fmt.Errorf("the API returned apiKeyWo: %s", raw)
		}

		return nil
	}
}

func TestInferenceEndpointKeyIsWrittenButNeverStored(t *testing.T) {
	fake := fakeapi.New(t)
	updated := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(inferenceEndpointAddress, plancheck.ResourceActionUpdate)},
	}
	neverStored := acctest.NothingContains(acctest.Value(inferenceKeyFirst), acctest.Value(inferenceKeySecond))

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "inference-endpoint", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: inferenceEndpointAt("https://api.openai.com/v1", "gpt-5", true, inferenceKeyFirst, 1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("address"), knownvalue.StringExact("acme")),
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("base_url"), knownvalue.StringExact("https://api.openai.com/v1")),
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("model"), knownvalue.StringExact("gpt-5")),
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("supports_reasoning_effort"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("api_key_wo"), knownvalue.Null()),
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("api_key_wo_version"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("api_key_set"), knownvalue.Bool(true)),
				},
				Check: resource.ComposeAggregateTestCheckFunc(inferenceEndpointStores(fake, inferenceKeyFirst), neverStored, inferenceEndpointAnswer(fake)),
			},
			{
				Config:   inferenceEndpointAt("https://api.openai.com/v1", "gpt-5", true, inferenceKeyFirst, 1),
				PlanOnly: true,
			},
			{
				Config:           inferenceEndpointKeepingKey("https://api.openai.com/v1", "gpt-5-mini", 1),
				ConfigPlanChecks: updated,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("model"), knownvalue.StringExact("gpt-5-mini")),
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("supports_reasoning_effort"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("api_key_set"), knownvalue.Bool(true)),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					inferenceEndpointStores(fake, inferenceKeyFirst),
					sentTimes(fake, inferenceEndpointKey, 1, "an update that omits the key sends no key"),
				),
			},
			{
				Config:           inferenceEndpointAt("https://api.openai.com/v1", "gpt-5-mini", false, inferenceKeySecond, 2),
				ConfigPlanChecks: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					inferenceEndpointStores(fake, inferenceKeySecond),
					sentTimes(fake, inferenceEndpointKey, 2, "a new api_key_wo_version sends the key again"),
					neverStored,
					inferenceEndpointAnswer(fake),
				),
			},
			{
				Config:           inferenceEndpointAt("https://api.openai.com/v1", "gpt-5-nano", false, inferenceKeyFirst, 2),
				ConfigPlanChecks: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					inferenceEndpointStores(fake, inferenceKeySecond),
					sentTimes(fake, inferenceEndpointKey, 2, "a new key with the same api_key_wo_version is not sent"),
				),
			},
			{
				ResourceName:      inferenceEndpointAddress,
				ImportState:       true,
				ImportStateIdFunc: inferenceEndpointImportID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					fake.Change(fake.IDOf("inference-endpoint", "acme"), func(fields map[string]any) { fields["model"] = "elsewhere" })
				},
				Config:           inferenceEndpointAt("https://api.openai.com/v1", "gpt-5-nano", false, inferenceKeyFirst, 2),
				ConfigPlanChecks: updated,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("model"), knownvalue.StringExact("gpt-5-nano")),
				},
				Check: sentTimes(fake, inferenceEndpointKey, 2, "putting back what changed sends no key"),
			},
		},
	})
}

func TestAnInferenceEndpointWithoutAKeyHoldsNone(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "inference-endpoint", "team", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: inferenceEndpointKeyless("http://localhost:11434/v1/", "llama3"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("base_url"), knownvalue.StringExact("http://localhost:11434/v1/")),
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("supports_reasoning_effort"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("api_key_wo_version"), knownvalue.Null()),
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("api_key_set"), knownvalue.Bool(false)),
				},
				Check: inferenceEndpointStores(fake, ""),
			},
			{
				Config:   inferenceEndpointKeyless("http://localhost:11434/v1/", "llama3"),
				PlanOnly: true,
			},
			{
				Config: inferenceEndpointAt("http://localhost:11434/v1/", "llama3", false, inferenceKeyFirst, 1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(inferenceEndpointAddress, tfjsonpath.New("api_key_set"), knownvalue.Bool(true)),
				},
				Check: inferenceEndpointStores(fake, inferenceKeyFirst),
			},
		},
	})
}

func TestSpotlightRejectsWhatAnInferenceEndpointCannotTake(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      inferenceEndpointAt("ftp://api.example.com", "gpt-5", false, inferenceKeyFirst, 1),
				ExpectError: regexp.MustCompile(`(?s)base_url: must start with http:// or https://`),
			},
			{
				Config:      inferenceEndpointAt("https://api.example.com/v1?x=1", "gpt-5", false, inferenceKeyFirst, 1),
				ExpectError: regexp.MustCompile(`(?s)base_url: must have no query or fragment`),
			},
			{
				Config:      inferenceEndpointAt("https://api.example.com/v1", "two words", false, inferenceKeyFirst, 1),
				ExpectError: regexp.MustCompile(`(?s)model: must be one name`),
			},
		},
	})
}

func TestAWorkspaceHasOneInferenceEndpoint(t *testing.T) {
	fake := fakeapi.New(t)
	twice := inferenceEndpointKeyless("https://api.example.com/v1", "gpt-5") + `
resource "spotlight_inference_endpoint" "again" {
  workspace_id = spotlight_workspace.acme.id
  base_url     = "https://api.example.com/v1"
  model        = "gpt-5"
}
`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:      twice,
				ExpectError: regexp.MustCompile(`(?s)There is already a inference-endpoint at acme`),
			},
		},
	})
}

func TestInferenceEndpointWritesNeedTheSecretsScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { fake.Grant("configure", "runners") },
				Config:      inferenceEndpointAt("https://api.example.com/v1", "gpt-5", false, inferenceKeyFirst, 1),
				ExpectError: regexp.MustCompile(`(?s)Not allowed to change the inference-endpoint.*lacks the secrets scope`),
			},
		},
	})
}

func TestInferenceEndpointImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            inferenceEndpointKeyless("https://api.example.com/v1", "gpt-5"),
				ConfigStateChecks: acctest.ExpectIdentity(inferenceEndpointAddress),
			},
			acctest.ImportedByIdentity(inferenceEndpointAddress),
		},
	})
}

func inferenceEndpointAccConfig(model string, version int) string {
	return fmt.Sprintf(`
variable "inference_api_key" {
  type      = string
  sensitive = true
}

resource "spotlight_inference_endpoint" "acc" {
  workspace_id       = local.workspace_id
  base_url           = "https://inference.example.com/v1"
  model              = %q
  api_key_wo         = var.inference_api_key
  api_key_wo_version = %d
}
`, model, version)
}

func inferenceEndpointAccVariables(key string) config.Variables {
	return config.Variables{"inference_api_key": config.StringVariable(key)}
}

func TestAccInferenceEndpoint(t *testing.T) {
	cp := acctest.PreCheck(t)
	address := "spotlight_inference_endpoint.acc"

	if _, err := cp.Client.ImportInferenceEndpoint(t.Context(), cp.Workspace.ID, cp.Workspace.Slug); !client.IsNotFound(err) {
		t.Skipf("the workspace %s already has an inference endpoint (%v); this test creates one only where nobody has", cp.Workspace.Slug, err)
	}

	k := acctest.RandomKeys()
	firstKey, secondKey := acctest.RandomToken(), acctest.RandomToken()
	cp.Sweep(t, k)
	t.Cleanup(func() {
		ctx := context.Background()
		if found, err := cp.Client.ImportInferenceEndpoint(ctx, cp.Workspace.ID, cp.Workspace.Slug); err == nil {
			if err := cp.Client.DeleteInferenceEndpoint(ctx, found.ID, found.Version); err != nil {
				t.Errorf("the inference endpoint of %s was left behind: %v", cp.Workspace.Slug, err)
			}
		}
	})

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy: func(state *terraform.State) error {
			if _, err := cp.Client.ImportInferenceEndpoint(context.Background(), cp.Workspace.ID, cp.Workspace.Slug); !client.IsNotFound(err) {
				return fmt.Errorf("the inference endpoint is still there (%v)", err)
			}

			return cp.NothingLeftButTheWorkspace(k)(state)
		},
		Steps: []resource.TestStep{
			{
				Config:          acctest.AdoptedWorkspace(cp.Workspace) + inferenceEndpointAccConfig("tf-acc-model", 1),
				ConfigVariables: inferenceEndpointAccVariables(firstKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "address", cp.Workspace.Slug),
					resource.TestCheckResourceAttr(address, "model", "tf-acc-model"),
					resource.TestCheckResourceAttr(address, "supports_reasoning_effort", "false"),
					resource.TestCheckResourceAttr(address, "api_key_set", "true"),
					resource.TestCheckNoResourceAttr(address, "api_key_wo"),
					acctest.NothingContains(acctest.Value(firstKey)),
				),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + inferenceEndpointAccConfig("tf-acc-model", 1),
				ConfigVariables:  inferenceEndpointAccVariables(firstKey),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config:          acctest.AdoptedWorkspace(cp.Workspace) + inferenceEndpointAccConfig("tf-acc-model-2", 2),
				ConfigVariables: inferenceEndpointAccVariables(secondKey),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "model", "tf-acc-model-2"),
					resource.TestCheckResourceAttr(address, "api_key_wo_version", "2"),
					acctest.NothingContains(acctest.Value(firstKey), acctest.Value(secondKey)),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     acctest.ImportID(cp.Workspace.ID, cp.Workspace.Slug),
				ImportStateVerify: true,
				ConfigVariables:   inferenceEndpointAccVariables(secondKey),
			},
			{
				Config:          acctest.ReleasedWorkspace(cp.Workspace) + inferenceEndpointAccConfig("tf-acc-model-2", 2),
				ConfigVariables: inferenceEndpointAccVariables(secondKey),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					acctest.ExpectForgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction(address, plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}
