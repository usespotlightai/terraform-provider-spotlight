package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
)

const (
	exportedWorkspaceID = "00000000-0000-4000-8000-00000000a0a0"
	exportedSecretValue = "lin_exported_never_kept"
)

const requiredProviders = `
terraform {
  required_providers {
    spotlight = {
      source = "usespotlightai/spotlight"
    }
  }
}
`

func TestAnExportAppliesFromScratchAndAdoptsWithNoDiff(t *testing.T) {
	t.Setenv(resource.EnvTfAccProviderNamespace, "usespotlightai")

	fake := fakeapi.New(t)

	api := acctest.FakeClient(t, fake)

	workspace, err := api.CreateWorkspace(t.Context(), client.WorkspaceInput{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatal(err)
	}

	dir, err := filepath.Abs("testdata/export")
	if err != nil {
		t.Fatal(err)
	}

	module, err := acctest.ModuleIn(dir)
	if err != nil {
		t.Fatal(err)
	}

	exported := strings.ReplaceAll(module.Config(dir), exportedWorkspaceID, workspace.ID)
	var created []string
	for _, address := range acctest.Addresses(exported) {
		if !strings.HasPrefix(address, "spotlight_workspace.") {
			created = append(created, address)
		}
	}

	fromScratch := acctest.WithoutImports(exported, created...)
	released := requiredProviders + acctest.Released(acctest.Addresses(exported))
	variables := config.Variables{
		"secret_linear_api_key":                    config.StringVariable(exportedSecretValue),
		"egress_shared_token":                      config.StringVariable(tokenThird),
		"runner_pool_k8s-jobs_provision_secret":    config.StringVariable(provisionFirst),
		"hook_core_linear-comments_signing_secret": config.StringVariable(provisionSecond),
	}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config:          fromScratch,
				ConfigVariables: variables,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{acctest.Adopts{Creating: created}},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					holdsOneOfEach(fake),
					acctest.NothingHolds(acctest.Value(exportedSecretValue), acctest.Value(tokenThird), acctest.Value(provisionFirst), acctest.Value(provisionSecond)),
					resource.TestCheckResourceAttr("spotlight_line.core_delivery", "station.1.profile", "careful"),
					resource.TestCheckResourceAttr("spotlight_line.core_delivery", "station.1.alert.1.falls_by", "0.1"),
					resource.TestCheckResourceAttr("spotlight_line.core_delivery", "station.1.alert.2.below", "0.4"),
					resource.TestCheckResourceAttr("spotlight_line.core_delivery", "replay_check.ceiling", "12.5"),
					resource.TestCheckResourceAttr("spotlight_line.core_delivery", "source.path", "teams/core/delivery.tf"),
					resource.TestCheckResourceAttr("spotlight_workspace_runners.acme", "trusts.0", "production-cluster"),
					resource.TestCheckResourceAttr("spotlight_workspace_runners.acme", "pools.#", "2"),
					resource.TestCheckResourceAttr("spotlight_runner_pool.burst", "token_version", "2"),
					resource.TestCheckResourceAttr("spotlight_egress.shared", "token_version", "2"),
					resource.TestCheckResourceAttrPair("spotlight_runner_pool.k8s-jobs", "provision.egress", "spotlight_egress.shared", "id"),
					resource.TestCheckResourceAttr("spotlight_hook.core_linear-comments", "match.tools.0", "linear.comment"),
					resource.TestCheckResourceAttr("spotlight_hook.core_merged", "signing.oidc.audience", "https://dba.acme.dev"),
				),
			},
			{
				Config:          exported,
				ConfigVariables: variables,
				PlanOnly:        true,
			},
			{
				Config: released,
			},
			{
				Config:          exported,
				ConfigVariables: variables,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{acctest.Adopts{}},
				},
				Check: holdsOneOfEach(fake),
			},
			{
				Config:          exported,
				ConfigVariables: variables,
				PlanOnly:        true,
			},
		},
	})
}

func holdsOneOfEach(fake *fakeapi.Server) resource.TestCheckFunc {
	return func(*terraform.State) error {
		for kind, want := range map[string]int{
			"workspace": 1, "secret": 2, "workspace-token-trust": 2, "runner-trust": 1, "egress": 1, "runner-pool": 2, "workspace-runners": 1,
			"team": 1, "line": 1, "skill": 1, "integration": 1, "harness-profile": 1, "hook": 2,
		} {
			if got := fake.Count(kind); got != want {
				return fmt.Errorf("Spotlight holds %d of %s, want %d", got, kind, want)
			}
		}

		return nil
	}
}

var exportedVariable = regexp.MustCompile(`(?m)^variable "([^"]+)" \{\n(?:  [^\n]*\n)*?\}`)

type exportUnavailable struct{ status int }

func (e exportUnavailable) Error() string {
	return fmt.Sprintf("the control plane answered %d", e.status)
}

func exportOf(ctx context.Context, cp acctest.ControlPlane) (acctest.Module, error) {
	endpoint := strings.TrimSuffix(strings.TrimRight(os.Getenv(provider.URLEnv), "/"), "/api")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/api/resources/workspaces/"+cp.Workspace.ID+"/export", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+os.Getenv(provider.TokenEnv))
	req.Header.Set("Spotlight-Client", client.ClientName("acc"))
	req.Header.Set("Spotlight-Api-Version", client.APIVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, exportUnavailable{resp.StatusCode}
	}

	var held struct {
		Files []struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&held); err != nil {
		return nil, err
	}

	module := acctest.Module{}
	for _, each := range held.Files {
		module[each.Path] = each.Content
	}

	return module, nil
}

func seedExported(t *testing.T, cp acctest.ControlPlane, k acctest.Keys) (declares bool) {
	t.Helper()

	ctx := t.Context()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	version := int64(1)
	value := acctest.RandomToken()
	_, err := cp.Client.CreateSecret(ctx, cp.Workspace.ID, client.SecretInput{
		Name:         k.Secret,
		Source:       client.SecretSource{Kind: client.SourceBuiltIn},
		Bindings:     []client.SecretBinding{{Host: "api.linear.app", Carrier: "header", Name: "authorization", Prefix: "Bearer "}},
		Value:        &value,
		ValueVersion: &version,
	})
	must(err)

	_, err = cp.Client.CreateSecret(ctx, cp.Workspace.ID, client.SecretInput{
		Name:     k.AWSSecret,
		Source:   client.SecretSource{Kind: client.SourceAWSSecretsManager, SecretID: "tf-acc/" + k.AWSSecret},
		Bindings: []client.SecretBinding{},
	})
	must(err)

	_, err = cp.Client.CreateRunnerTrust(ctx, cp.Workspace.ID, client.RunnerTrustInput{
		Key: k.Trust, Issuer: "https://oidc.eks.eu-west-1.amazonaws.com/id/TFACC", Subject: "system:serviceaccount:tf-acc:runner", Audience: "spotlight", Labels: []string{"tf-acc-gpu"},
	})
	must(err)

	tokenVersion, secretVersion := int64(1), int64(1)
	maxRuns := int64(2)
	_, err = cp.Client.CreateRunnerPool(ctx, cp.Workspace.ID, client.RunnerPoolInput{
		Key: k.Pool, Labels: []string{"tf-acc-linux"}, MaxRuns: &maxRuns, Token: acctest.RandomToken(), TokenVersion: &tokenVersion,
	})
	must(err)

	egressVersion := int64(1)
	egress, err := cp.Client.CreateEgress(ctx, cp.Workspace.ID, client.EgressInput{
		Key: k.Egress, URL: "https://egress.example.com", Token: acctest.RandomToken(), TokenVersion: &egressVersion,
	})
	must(err)

	_, err = cp.Client.CreateRunnerPool(ctx, cp.Workspace.ID, client.RunnerPoolInput{
		Key: k.PushPool, Labels: []string{},
		Provision: &client.PoolProvision{URL: "https://provision.example.com/runs", Secret: acctest.RandomSigningSecret(), SecretVersion: &secretVersion, Egress: &egress.ID},
	})
	must(err)

	team, err := cp.Client.CreateTeam(ctx, cp.Workspace.ID, client.TeamInput{Key: k.Team, Name: "Terraform acceptance " + k.Team})
	must(err)

	_, err = cp.Client.CreateHarnessProfile(ctx, team.ID, client.HarnessProfileInput{
		Key:         k.Profile,
		Permissions: client.Permissions{Allow: []string{"read"}, Ask: []string{}, Deny: []string{"read(.env)"}},
		MCPServers:  []client.MCPServer{{Name: "linear", Command: "npx", Args: []string{"-y", "linear-mcp"}, Env: map[string]string{"LINEAR_WORKSPACE": "tf-acc"}}},
		Native:      map[string]any{"claude-code": map[string]any{"includeCoAuthoredBy": false}},
	})
	must(err)

	_, err = cp.Client.CreateSkill(ctx, team.ID, client.SkillInput{Key: k.Skill, Files: map[string]string{
		"SKILL.md": "---\nname: " + k.Skill + "\ndescription: Look up dashboards\n---\nSearch before you guess.\n",
	}})
	must(err)

	_, err = cp.Client.CreateIntegration(ctx, team.ID, client.Definition{
		"integration": k.Integration,
		"actions": map[string]any{"comment": map[string]any{"request": map[string]any{
			"method": "POST", "url": "https://api.linear.app/graphql",
			"headers": map[string]any{"Content-Type": "application/json"},
			"body":    map[string]any{"query": "${{ inputs.body }}"},
			"auth":    map[string]any{"secrets": []any{"secret." + k.Secret}},
		}}},
		"tools": map[string]any{"comment": map[string]any{
			"does": "comment", "description": "Comment on the issue.\n",
			"inputs": map[string]any{"body": map[string]any{"type": "string", "required": true}},
		}},
	})
	must(err)

	signingVersion := int64(1)
	_, err = cp.Client.CreateHook(ctx, team.ID, client.HookInput{
		Key:   k.Hook,
		On:    "tool.used",
		Match: client.HookMatch{Tools: []string{k.Integration + ".comment"}},
		Vars:  map[string]string{"channel": "C024BE91L"},
		Steps: []any{
			map[string]any{"name": "note", "call": k.Integration + ".comment", "with": map[string]any{"body": "Commented: ${{ event.inputs.body }}"}},
			map[string]any{"name": "audit", "request": map[string]any{"url": "https://audit.acme.dev/hooks", "body": map[string]any{"tool": "${{ event.tool }}"}, "from": "network"}},
		},
		Signing: &client.HookSigning{Secret: acctest.RandomSigningSecret(), SecretVersion: &signingVersion},
	})
	must(err)

	_, err = cp.Client.CreateLine(ctx, team.ID, client.Definition{
		"key": k.Line, "name": "Review", "role": "review", "runners": []any{"tf-acc-linux"},
		"repositories": []any{map[string]any{"name": "acme/api", "image": "ghcr.io/acme/api-runner:main", "setup": "npm ci"}},
		"stations": []any{
			map[string]any{"key": "backlog", "name": "Backlog", "kind": "backlog", "passesTo": []any{"implementation"}},
			map[string]any{
				"key": "implementation", "name": "Implementation", "harness": "claude-code",
				"profile": k.Profile, "prompt": "Implement the task.\n", "skills": []any{k.Skill},
				"tools": []any{k.Integration + ".comment"}, "runners": []any{"tf-acc-gpu"}, "passesTo": []any{"code-review"},
			},
			map[string]any{"key": "code-review", "name": "Code review", "harness": "claude-code", "prompt": "Review the change.\n", "passesTo": []any{"implementation"}, "ends": true},
			map[string]any{"key": "completed", "name": "Completed", "kind": "completed"},
		},
		"reviews": map[string]any{"start": "opened", "skip": map[string]any{"authors": []any{"renovate[bot]", "dependabot[bot]"}}},
	})
	must(err)

	if _, err := cp.DeclaredRunners(ctx); !client.IsNotFound(err) {
		return false
	}

	_, err = cp.Client.CreateWorkspaceRunners(ctx, cp.Workspace.ID, client.WorkspaceRunnersInput{Trusts: []string{k.Trust}, Pools: []string{k.Pool, k.PushPool}})
	must(err)

	return true
}

func forgetSeeded(t *testing.T, cp acctest.ControlPlane, k acctest.Keys, declares bool) {
	t.Helper()

	if declares {
		if err := cp.Undeclare(t.Context()); err != nil {
			t.Fatalf("could not delete the declaration of the workspace's runners to declare them again from the export: %v", err)
		}
	}

	if err := cp.RemoveCreated(t.Context(), k); err != nil {
		t.Fatalf("could not delete what the test seeded to create it again from the export: %v", err)
	}
}

func createdFromScratch(imports map[string]string, workspace client.Workspace, k acctest.Keys, declares bool) []string {
	names := []string{k.Secret, k.AWSSecret, k.Trust, k.Egress, k.Pool, k.PushPool, k.Team}
	if declares {
		names = append(names, workspace.Slug)
	}

	var out []string
	for address, id := range imports {
		within, ok := strings.CutPrefix(id, workspace.ID+"/")
		if !ok {
			continue
		}

		if slices.Contains(names, within) || strings.HasPrefix(within, k.Team+"/") {
			out = append(out, address)
		}
	}
	slices.Sort(out)

	return out
}

func adoptedWorkspaceProtected(config string) string {
	for _, address := range acctest.Addresses(config) {
		if strings.HasPrefix(address, "spotlight_workspace.") {
			config = acctest.Protected(config, address)
		}
	}

	return config
}

func TestAccExportAppliesAndReplansWithNoDiff(t *testing.T) {
	cp := acctest.PreCheck(t)

	if _, err := exportOf(t.Context(), cp); err != nil {
		var unavailable exportUnavailable
		if errors.As(err, &unavailable) && unavailable.status == http.StatusNotFound {
			t.Skipf("this control plane serves no export (%v): it runs a Spotlight from before SPO-80", err)
		}
		t.Fatalf("could not export the workspace: %v", err)
	}

	t.Setenv(resource.EnvTfAccProviderNamespace, "usespotlightai")

	k := acctest.RandomKeys()
	cp.Sweep(t, k)
	declares := false
	t.Cleanup(func() {
		if declares {
			if err := cp.Undeclare(context.Background()); err != nil {
				t.Errorf("the declaration of %s's runners was left behind: %v", cp.Workspace.Slug, err)
			}
		}
	})
	declares = seedExported(t, cp, k)

	module, err := exportOf(t.Context(), cp)
	if err != nil {
		t.Fatal(err)
	}

	forgetSeeded(t, cp, k, declares)

	dir := t.TempDir()
	if err := module.WriteTo(dir); err != nil {
		t.Fatal(err)
	}

	exported := adoptedWorkspaceProtected(module.Config(dir))
	created := createdFromScratch(acctest.Imports(exported), cp.Workspace, k, declares)
	t.Logf("the export imports %d resources, %d of them created by this test from scratch: %v", len(acctest.Imports(exported)), len(created), created)

	variables := config.Variables{}
	var values []func() string
	for _, found := range exportedVariable.FindAllStringSubmatch(module["variables.tf"], -1) {
		if !strings.Contains(found[0], "default") {
			value := acctest.RandomToken()
			if strings.HasSuffix(found[1], "_provision_secret") || strings.HasSuffix(found[1], "_signing_secret") {
				value = acctest.RandomSigningSecret()
			}
			variables[found[1]] = config.StringVariable(value)
			values = append(values, acctest.Value(value))
		}
	}

	released := requiredProviders + acctest.Released(acctest.Addresses(exported))

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnly,
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             cp.NothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config:          acctest.WithoutImports(exported, created...),
				ConfigVariables: variables,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{acctest.Adopts{Creating: created}},
				},
				Check: acctest.NothingHolds(values...),
			},
			{
				Config:           exported,
				ConfigVariables:  variables,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config: released,
				Check:  removeExported(cp, k, &declares),
			},
		},
	})
}

func removeExported(cp acctest.ControlPlane, k acctest.Keys, declares *bool) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()

		if *declares {
			if err := cp.Undeclare(ctx); err != nil {
				return fmt.Errorf("could not delete the declaration of the workspace's runners: %w", err)
			}
			*declares = false
		}

		if err := cp.RemoveCreated(ctx, k); err != nil {
			return fmt.Errorf("could not delete what the test created: %w", err)
		}

		return nil
	}
}
