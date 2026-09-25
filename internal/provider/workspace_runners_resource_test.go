package provider_test

import (
	"context"
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

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

const workspaceRunnersAddress = "spotlight_workspace_runners.acme"

const workspaceRunnersNeighbours = `
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

resource "spotlight_runner_pool" "ephemeral" {
  workspace_id = spotlight_workspace.acme.id
  key          = "ephemeral"
  token        = "` + "tok_pool_ephemeral_0123456789abcdefgh" + `"
}
`

func workspaceRunnersDeclared(runners, trusts, pools string) string {
	return acctest.Workspace + workspaceRunnersNeighbours + fmt.Sprintf(`
resource "spotlight_workspace_runners" "acme" {
  workspace_id = spotlight_workspace.acme.id
  runners      = [%s]
  trusts       = [%s]
  pools        = [%s]
}
`, runners, trusts, pools)
}

func workspaceRunnersImportID(state *terraform.State) (string, error) {
	return state.RootModule().Resources["spotlight_workspace.acme"].Primary.ID + "/acme", nil
}

func workspaceRunnersNames(values ...string) []knownvalue.Check {
	out := make([]knownvalue.Check, 0, len(values))
	for _, each := range values {
		out = append(out, knownvalue.StringExact(each))
	}

	return out
}

func TestWorkspaceRunnersDeclaredAndDrift(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		CheckDestroy:             acctest.NoneLeft(fake, "workspace-runners", "runner", "runner-trust", "runner-pool", "workspace"),
		Steps: []resource.TestStep{
			{
				Config: workspaceRunnersDeclared("spotlight_runner.build.name", "spotlight_runner_trust.cluster.key", "spotlight_runner_pool.ephemeral.key"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(workspaceRunnersAddress, tfjsonpath.New("address"), knownvalue.StringExact("acme")),
					statecheck.ExpectKnownValue(workspaceRunnersAddress, tfjsonpath.New("runners"), knownvalue.SetExact(workspaceRunnersNames("build-1"))),
					statecheck.ExpectKnownValue(workspaceRunnersAddress, tfjsonpath.New("trusts"), knownvalue.SetExact(workspaceRunnersNames("production-cluster"))),
					statecheck.ExpectKnownValue(workspaceRunnersAddress, tfjsonpath.New("pools"), knownvalue.SetExact(workspaceRunnersNames("ephemeral"))),
					statecheck.ExpectKnownValue(workspaceRunnersAddress, tfjsonpath.New("drift"), knownvalue.SetSizeExact(0)),
				},
			},
			{
				Config:   workspaceRunnersDeclared("spotlight_runner.build.name", "spotlight_runner_trust.cluster.key", "spotlight_runner_pool.ephemeral.key"),
				PlanOnly: true,
			},
			{
				ResourceName:      workspaceRunnersAddress,
				ImportState:       true,
				ImportStateIdFunc: workspaceRunnersImportID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					fake.Enrol(fake.IDOf("workspace", "acme"), "stray-1", "")
					fake.Enrol(fake.IDOf("workspace", "acme"), "pod-7", "production-cluster")
				},
				Config: workspaceRunnersDeclared("spotlight_runner.build.name", "spotlight_runner_trust.cluster.key", "spotlight_runner_pool.ephemeral.key"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(workspaceRunnersAddress, tfjsonpath.New("drift"), knownvalue.SetExact(workspaceRunnersNames("stray-1"))),
				},
			},
			{
				Config: workspaceRunnersDeclared(`spotlight_runner.build.name, "stray-1"`, "spotlight_runner_trust.cluster.key", "spotlight_runner_pool.ephemeral.key"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceRunnersAddress, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(workspaceRunnersAddress, tfjsonpath.New("drift"), knownvalue.SetSizeExact(0)),
				},
			},
			{
				PreConfig: func() {
					fake.Forget(fake.IDOf("runner", "stray-1"))
					fake.Forget(fake.IDOf("runner", "pod-7"))
					fake.Change(fake.IDOf("workspace-runners", "acme"), func(fields map[string]any) {
						fields["trusts"] = []any{}
						fields["pools"] = []any{}
					})
				},
				Config: workspaceRunnersDeclared(`spotlight_runner.build.name, "stray-1"`, "spotlight_runner_trust.cluster.key", "spotlight_runner_pool.ephemeral.key"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workspaceRunnersAddress, plancheck.ResourceActionUpdate)},
				},
			},
		},
	})
}

func TestAWorkspaceDeclaresItsRunnersOnce(t *testing.T) {
	fake := fakeapi.New(t)
	twice := strings.Replace(workspaceRunnersDeclared("", "", ""), `resource "spotlight_workspace_runners" "acme"`, `resource "spotlight_workspace_runners" "again" {
  workspace_id = spotlight_workspace.acme.id
}

resource "spotlight_workspace_runners" "acme"`, 1)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
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
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				PreConfig: func() { fake.Grant("configure", "secrets") },
				Config: acctest.Workspace + `
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

	api := acctest.FakeClient(t, fake)

	workspace, err := api.CreateWorkspace(ctx, client.WorkspaceInput{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatal(err)
	}

	declaration, err := api.CreateWorkspaceRunners(ctx, workspace.ID, client.WorkspaceRunnersInput{Runners: []string{"build-1"}, Pools: []string{"burst"}})
	if err != nil {
		t.Fatal(err)
	}

	fake.Enrol(workspace.ID, "stray-1", "")

	server, schemas := acctest.ConfiguredServer(t, fake)

	kind := schemas.ResourceSchemas["spotlight_workspace_runners"].ValueType()
	names := tftypes.Set{ElementType: tftypes.String}
	held, err := tfprotov6.NewDynamicValue(kind, tftypes.NewValue(kind, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, declaration.ID),
		"version":      tftypes.NewValue(tftypes.Number, declaration.Version),
		"address":      tftypes.NewValue(tftypes.String, "acme"),
		"workspace_id": tftypes.NewValue(tftypes.String, workspace.ID),
		"runners":      tftypes.NewValue(names, []tftypes.Value{tftypes.NewValue(tftypes.String, "build-1")}),
		"trusts":       tftypes.NewValue(names, []tftypes.Value{}),
		"pools":        tftypes.NewValue(names, []tftypes.Value{tftypes.NewValue(tftypes.String, "burst")}),
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
	for _, want := range []string{"names build-1, but no runner by that name is enrolled", "leaves out stray-1, which is enrolled all the same", "names burst, but the workspace has no runner pool by that key"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the read did not warn %q; it said:\n%s", want, joined)
		}
	}
}

func workspaceRunnersAccConfig(k acctest.Keys, withTrust bool) string {
	trusts := ""
	if withTrust {
		trusts = "spotlight_runner_trust.cluster.key"
	}

	return runnerAccConfig(k, `"tf-acc-gpu"`, 1) + fmt.Sprintf(`
resource "spotlight_workspace_runners" "acc" {
  workspace_id = local.workspace_id
  runners      = [spotlight_runner.build.name]
  trusts       = [%s]
}
`, trusts)
}
func TestAccWorkspaceRunners(t *testing.T) {
	cp := acctest.PreCheck(t)

	if _, err := cp.DeclaredRunners(t.Context()); !client.IsNotFound(err) {
		t.Skipf("the workspace %s already declares its runners (%v); this test declares them only where nobody has", cp.Workspace.Slug, err)
	}

	k := acctest.RandomKeys()
	token := acctest.RandomToken()
	cp.Sweep(t, k)
	t.Cleanup(func() {
		ctx := context.Background()
		if held, err := cp.DeclaredRunners(ctx); err == nil {
			if err := cp.Client.DeleteWorkspaceRunners(ctx, held.ID, held.Version); err != nil {
				t.Errorf("the declaration of %s's runners was left behind: %v", cp.Workspace.Slug, err)
			}
		}
	})

	nothingDeclared := func(state *terraform.State) error {
		if _, err := cp.DeclaredRunners(context.Background()); !client.IsNotFound(err) {
			return fmt.Errorf("the declaration of the workspace's runners is still there (%v)", err)
		}

		return cp.NothingLeftButTheWorkspace(k)(state)
	}

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             nothingDeclared,
		Steps: []resource.TestStep{
			{
				Config:          acctest.AdoptedWorkspace(cp.Workspace) + workspaceRunnersAccConfig(k, false),
				ConfigVariables: runnerAccVariables(token),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("spotlight_workspace_runners.acc", "address", cp.Workspace.Slug),
					resource.TestCheckResourceAttr("spotlight_workspace_runners.acc", "runners.#", "1"),
					resource.TestCheckResourceAttr("spotlight_workspace_runners.acc", "runners.0", k.Runner),
				),
			},
			{
				Config:           acctest.AdoptedWorkspace(cp.Workspace) + workspaceRunnersAccConfig(k, false),
				ConfigVariables:  runnerAccVariables(token),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config:          acctest.AdoptedWorkspace(cp.Workspace) + workspaceRunnersAccConfig(k, true),
				ConfigVariables: runnerAccVariables(token),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("spotlight_workspace_runners.acc", plancheck.ResourceActionUpdate),
				}},
				Check: resource.TestCheckResourceAttr("spotlight_workspace_runners.acc", "trusts.0", k.Trust),
			},
			{
				ResourceName:      "spotlight_workspace_runners.acc",
				ImportState:       true,
				ImportStateId:     acctest.ImportID(cp.Workspace.ID, cp.Workspace.Slug),
				ImportStateVerify: true,
				ConfigVariables:   runnerAccVariables(token),
			},
			{
				Config:          acctest.ReleasedWorkspace(cp.Workspace) + workspaceRunnersAccConfig(k, true),
				ConfigVariables: runnerAccVariables(token),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					acctest.ExpectForgotten("spotlight_workspace.acc"),
					plancheck.ExpectResourceAction("spotlight_workspace_runners.acc", plancheck.ResourceActionNoop),
				}},
			},
		},
	})
}

func TestWorkspaceRunnersImportsByIdentity(t *testing.T) {
	fake := fakeapi.New(t)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.Identity,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:            workspaceRunnersDeclared("spotlight_runner.build.name", "spotlight_runner_trust.cluster.key", "spotlight_runner_pool.ephemeral.key"),
				ConfigStateChecks: acctest.ExpectIdentity(workspaceRunnersAddress),
			},
			acctest.ImportedByIdentity(workspaceRunnersAddress),
		},
	})
}
