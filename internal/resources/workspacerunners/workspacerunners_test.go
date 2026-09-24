package workspacerunners_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/providertest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const address = "spotlight_workspace_runners.acme"

const neighbours = `
resource "spotlight_runner" "build" {
  workspace_id = spotlight_workspace.acme.id
  name         = "build-1"
  token        = "` + "tok_build_one_0123456789abcdefghijklmn" + `"
}

resource "spotlight_runner_trust" "cluster" {
  workspace_id = spotlight_workspace.acme.id
  key          = "production-cluster"
  issuer       = "https://oidc.example.com"
  subject      = "system:serviceaccount:spotlight:runner"
}
`

func declared(runners, trusts string) string {
	return providertest.Workspace + neighbours + fmt.Sprintf(`
resource "spotlight_workspace_runners" "acme" {
  workspace_id = spotlight_workspace.acme.id
  runners      = [%s]
  trusts       = [%s]
}
`, runners, trusts)
}

func importID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/acme", nil
}

func names(values ...string) []knownvalue.Check {
	out := make([]knownvalue.Check, 0, len(values))
	for _, each := range values {
		out = append(out, knownvalue.StringExact(each))
	}

	return out
}

func TestWorkspaceRunnersDeclaredAndDrift(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		ProtoV6ProviderFactories: providertest.Factories(fake),
		CheckDestroy:             providertest.NoneLeft(fake, "workspace-runners", "runner", "runner-trust", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: declared("spotlight_runner.build.name", "spotlight_runner_trust.cluster.key"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("address"), knownvalue.StringExact("acme")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("runners"), knownvalue.SetExact(names("build-1"))),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("trusts"), knownvalue.SetExact(names("production-cluster"))),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("drift"), knownvalue.SetSizeExact(0)),
				},
			},
			{
				Config:   declared("spotlight_runner.build.name", "spotlight_runner_trust.cluster.key"),
				PlanOnly: true,
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateIdFunc: importID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					fake.Enrol(fake.IDOf("workspace", "acme"), "stray-1", "")
					fake.Enrol(fake.IDOf("workspace", "acme"), "pod-7", "production-cluster")
				},
				Config: declared("spotlight_runner.build.name", "spotlight_runner_trust.cluster.key"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("drift"), knownvalue.SetExact(names("stray-1"))),
				},
			},
			{
				Config: declared(`spotlight_runner.build.name, "stray-1"`, "spotlight_runner_trust.cluster.key"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("drift"), knownvalue.SetSizeExact(0)),
				},
			},
			{
				PreConfig: func() {
					fake.Forget(fake.IDOf("runner", "stray-1"))
					fake.Forget(fake.IDOf("runner", "pod-7"))
					fake.Change(fake.IDOf("workspace-runners", "acme"), func(fields map[string]any) { fields["trusts"] = []any{} })
				},
				Config: declared(`spotlight_runner.build.name, "stray-1"`, "spotlight_runner_trust.cluster.key"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
				},
			},
		},
	})
}

func TestAWorkspaceDeclaresItsRunnersOnce(t *testing.T) {
	fake := fakeapi.New(t)
	twice := strings.Replace(declared("", ""), `resource "spotlight_workspace_runners" "acme"`, `resource "spotlight_workspace_runners" "again" {
  workspace_id = spotlight_workspace.acme.id
}

resource "spotlight_workspace_runners" "acme"`, 1)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				Config:      twice,
				ExpectError: regexp.MustCompile(`(?s)There is already a workspace-runners at acme.*Import it`),
			},
		},
	})
}

func TestDeclaringRunnersNeedsTheRunnersScope(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providertest.Factories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig: func() { fake.Grant("configure", "secrets") },
				Config: providertest.Workspace + `
resource "spotlight_workspace_runners" "acme" {
  workspace_id = spotlight_workspace.acme.id
  runners      = ["build-1"]
}
`,
				ExpectError: regexp.MustCompile(`(?s)lacks the runners scope`),
			},
		},
	})
}

func TestEveryReadWarnsOfDrift(t *testing.T) {
	fake := fakeapi.New(t)
	ctx := t.Context()

	api, err := client.New(client.Config{Endpoint: fake.URL, Token: fakeapi.Token})
	if err != nil {
		t.Fatal(err)
	}

	workspace, err := api.Workspaces.Create(ctx, "", spotlight.WorkspaceInput{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatal(err)
	}

	declaration, err := api.WorkspaceRunners.Create(ctx, workspace.ID, spotlight.WorkspaceRunnersInput{Runners: []string{"build-1"}})
	if err != nil {
		t.Fatal(err)
	}

	fake.Enrol(workspace.ID, "stray-1", "")

	server, err := providertest.Factories(fake)["spotlight"]()
	if err != nil {
		t.Fatal(err)
	}

	schemas, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}

	providerConfig, err := tfprotov6.NewDynamicValue(schemas.Provider.ValueType(), tftypes.NewValue(schemas.Provider.ValueType(), map[string]tftypes.Value{
		"endpoint": tftypes.NewValue(tftypes.String, nil),
		"token":    tftypes.NewValue(tftypes.String, nil),
	}))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := server.ConfigureProvider(ctx, &tfprotov6.ConfigureProviderRequest{Config: &providerConfig}); err != nil {
		t.Fatal(err)
	}

	kind := schemas.ResourceSchemas["spotlight_workspace_runners"].ValueType()
	names := tftypes.Set{ElementType: tftypes.String}
	held, err := tfprotov6.NewDynamicValue(kind, tftypes.NewValue(kind, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, declaration.ID),
		"version":      tftypes.NewValue(tftypes.Number, declaration.Version),
		"address":      tftypes.NewValue(tftypes.String, "acme"),
		"workspace_id": tftypes.NewValue(tftypes.String, workspace.ID),
		"runners":      tftypes.NewValue(names, []tftypes.Value{tftypes.NewValue(tftypes.String, "build-1")}),
		"trusts":       tftypes.NewValue(names, []tftypes.Value{}),
		"drift":        tftypes.NewValue(names, []tftypes.Value{}),
	}))
	if err != nil {
		t.Fatal(err)
	}

	read, err := server.ReadResource(ctx, &tfprotov6.ReadResourceRequest{TypeName: "spotlight_workspace_runners", CurrentState: &held})
	if err != nil {
		t.Fatal(err)
	}

	var said []string
	for _, each := range read.Diagnostics {
		if each.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("read failed: %s: %s", each.Summary, each.Detail)
		}
		said = append(said, each.Detail)
	}

	joined := strings.Join(said, "\n")
	for _, want := range []string{"names build-1, but no runner by that name is enrolled", "leaves out stray-1, which is enrolled all the same"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the read did not warn %q; it said:\n%s", want, joined)
		}
	}
}
