package acctest_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/providertest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
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

var needsValue = regexp.MustCompile(`(?m)^variable "([^"]+)" \{\n(?:  [^\n]*\n)*?\}`)

type exportUnavailable struct{ status int }

func (e exportUnavailable) Error() string {
	return fmt.Sprintf("the control plane answered %d", e.status)
}

func (cp controlPlane) export(ctx context.Context) (providertest.Module, error) {
	endpoint := strings.TrimSuffix(strings.TrimRight(os.Getenv(provider.EndpointEnv), "/"), "/api")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/api/resources/workspaces/"+cp.workspace.ID+"/export", nil)
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

	module := providertest.Module{}
	for _, each := range held.Files {
		module[each.Path] = each.Content
	}

	return module, nil
}

func (cp controlPlane) seed(t *testing.T, k keys) (declares bool) {
	t.Helper()

	ctx := t.Context()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	version := int64(1)
	value := randomToken()
	_, err := cp.api.Secrets.Create(ctx, cp.workspace.ID, spotlight.SecretInput{
		Name:         k.secret,
		Source:       spotlight.SecretSource{Kind: spotlight.SourceBuiltIn},
		Bindings:     []spotlight.SecretBinding{{Host: "api.linear.app", Header: "authorization", Prefix: "Bearer "}},
		Value:        &value,
		ValueVersion: &version,
	})
	must(err)

	_, err = cp.api.Secrets.Create(ctx, cp.workspace.ID, spotlight.SecretInput{
		Name:     k.awsSecret,
		Source:   spotlight.SecretSource{Kind: spotlight.SourceAWSSecretsManager, SecretID: "tf-acc/" + k.awsSecret},
		Bindings: []spotlight.SecretBinding{},
	})
	must(err)

	_, err = cp.api.Runners.Create(ctx, cp.workspace.ID, spotlight.RunnerInput{Name: k.runner, Labels: []string{"tf-acc-vpc"}, Token: randomToken()})
	must(err)

	_, err = cp.api.RunnerTrusts.Create(ctx, cp.workspace.ID, spotlight.RunnerTrustInput{
		Key: k.trust, Issuer: "https://oidc.eks.eu-west-1.amazonaws.com/id/TFACC", Subject: "system:serviceaccount:tf-acc:runner", Audience: "spotlight", Labels: []string{"tf-acc-gpu"},
	})
	must(err)

	team, err := cp.api.Teams.Create(ctx, cp.workspace.ID, spotlight.TeamInput{Key: k.team, Name: "Terraform acceptance " + k.team})
	must(err)

	_, err = cp.api.HarnessProfiles.Create(ctx, team.ID, spotlight.HarnessProfileInput{
		Key:         k.profile,
		Permissions: spotlight.Permissions{Allow: []string{"read"}, Ask: []string{}, Deny: []string{"read(.env)"}},
		MCPServers:  []spotlight.MCPServer{{Name: "linear", Command: "npx", Args: []string{"-y", "linear-mcp"}, Env: map[string]string{"LINEAR_WORKSPACE": "tf-acc"}}},
		Native:      map[string]any{"claude-code": map[string]any{"includeCoAuthoredBy": false}},
	})
	must(err)

	_, err = cp.api.Skills.Create(ctx, team.ID, spotlight.SkillInput{Key: k.skill, Files: map[string]string{
		"SKILL.md": "---\nname: " + k.skill + "\ndescription: Look up dashboards\n---\nSearch before you guess.\n",
	}})
	must(err)

	_, err = cp.api.Integrations.Create(ctx, team.ID, spotlight.Definition{
		"integration": k.integration,
		"actions": map[string]any{"comment": map[string]any{"request": map[string]any{
			"method": "POST", "url": "https://api.linear.app/graphql",
			"headers": map[string]any{"Content-Type": "application/json"},
			"body":    map[string]any{"query": "${{ inputs.body }}"},
			"auth":    map[string]any{"secrets": []any{"secret." + k.secret}},
		}}},
		"tools": map[string]any{"comment": map[string]any{
			"does": "comment", "description": "Comment on the issue.\n",
			"inputs": map[string]any{"body": map[string]any{"type": "string", "required": true}},
		}},
	})
	must(err)

	_, err = cp.api.Lines.Create(ctx, team.ID, spotlight.Definition{
		"key": k.line, "name": "Review", "role": "review", "runners": []any{k.runner, "tf-acc-vpc"},
		"stations": []any{
			map[string]any{"key": "backlog", "name": "Backlog", "kind": "backlog", "passesTo": []any{"implementation"}},
			map[string]any{
				"key": "implementation", "name": "Implementation", "harness": "claude-code", "harnessVersion": "2.1.270",
				"profile": k.profile, "prompt": "Implement the task.\n", "skills": []any{k.skill},
				"tools": []any{k.integration + ".comment"}, "runners": []any{"tf-acc-gpu"}, "passesTo": []any{"code-review"},
			},
			map[string]any{"key": "code-review", "name": "Code review", "harness": "claude-code", "prompt": "Review the change.\n", "passesTo": []any{"implementation"}, "ends": true},
			map[string]any{"key": "completed", "name": "Completed", "kind": "completed"},
		},
		"reviews": map[string]any{"start": "opened", "skip": map[string]any{"authors": []any{"renovate[bot]", "dependabot[bot]"}}},
	})
	must(err)

	if _, err := cp.declared(ctx); !spotlight.IsNotFound(err) {
		return false
	}

	_, err = cp.api.WorkspaceRunners.Create(ctx, cp.workspace.ID, spotlight.WorkspaceRunnersInput{Runners: []string{k.runner}, Trusts: []string{k.trust}})
	must(err)

	return true
}

func (cp controlPlane) undeclare(ctx context.Context) error {
	held, err := cp.declared(ctx)
	if spotlight.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	return cp.api.WorkspaceRunners.Delete(ctx, held.ID, held.Version)
}

func (cp controlPlane) forgetSeeded(t *testing.T, k keys, declares bool) {
	t.Helper()

	if declares {
		if err := cp.undeclare(t.Context()); err != nil {
			t.Fatalf("could not delete the declaration of the workspace's runners to declare them again from the export: %v", err)
		}
	}

	for _, each := range cp.created(k) {
		meta, err := each.find(t.Context())
		if spotlight.IsNotFound(err) {
			continue
		}
		if err == nil {
			err = each.remove(t.Context(), meta)
		}
		if err != nil {
			t.Fatalf("could not delete the %s %s to create it again from the export: %v", each.kind, each.address, err)
		}
	}
}

func ours(imports map[string]string, workspace spotlight.Workspace, k keys, declares bool) []string {
	names := []string{k.secret, k.awsSecret, k.runner, k.trust, k.team}
	if declares {
		names = append(names, workspace.Slug)
	}

	var out []string
	for address, id := range imports {
		within, ok := strings.CutPrefix(id, workspace.ID+"/")
		if !ok {
			continue
		}

		if slices.Contains(names, within) || strings.HasPrefix(within, k.team+"/") {
			out = append(out, address)
		}
	}
	slices.Sort(out)

	return out
}

func adoptedWorkspaceProtected(config string) string {
	for _, address := range providertest.Addresses(config) {
		if strings.HasPrefix(address, "spotlight_workspace.") {
			config = providertest.Protected(config, address)
		}
	}

	return config
}

func TestAccExportAppliesAndReplansWithNoDiff(t *testing.T) {
	cp := needsControlPlane(t)

	if _, err := cp.export(t.Context()); err != nil {
		var unavailable exportUnavailable
		if errors.As(err, &unavailable) && unavailable.status == http.StatusNotFound {
			t.Skipf("this control plane serves no export (%v): it runs a Spotlight from before SPO-80", err)
		}
		t.Fatalf("could not export the workspace: %v", err)
	}

	t.Setenv(resource.EnvTfAccProviderNamespace, "usespotlightai")

	k := randomKeys()
	cp.sweep(t, k)
	declares := false
	t.Cleanup(func() {
		if declares {
			if err := cp.undeclare(context.Background()); err != nil {
				t.Errorf("the declaration of %s's runners was left behind: %v", cp.workspace.Slug, err)
			}
		}
	})
	declares = cp.seed(t, k)

	module, err := cp.export(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	cp.forgetSeeded(t, k, declares)

	dir := t.TempDir()
	if err := module.WriteTo(dir); err != nil {
		t.Fatal(err)
	}

	exported := adoptedWorkspaceProtected(module.Config(dir))
	created := ours(providertest.Imports(exported), cp.workspace, k, declares)
	t.Logf("the export imports %d resources, %d of them created by this test from scratch: %v", len(providertest.Imports(exported)), len(created), created)

	variables := config.Variables{}
	var values []func() string
	for _, found := range needsValue.FindAllStringSubmatch(module["variables.tf"], -1) {
		if !strings.Contains(found[0], "default") {
			value := randomToken()
			variables[found[1]] = config.StringVariable(value)
			values = append(values, providertest.Value(value))
		}
	}

	released := requiredProviders + providertest.Released(providertest.Addresses(exported))

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   writeOnly,
		ProtoV6ProviderFactories: factories(),
		CheckDestroy:             cp.nothingLeftButTheWorkspace(k),
		Steps: []resource.TestStep{
			{
				Config:          providertest.WithoutImports(exported, created...),
				ConfigVariables: variables,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{providertest.Adopts{Creating: created}},
				},
				Check: providertest.NothingHolds(values...),
			},
			{
				Config:           exported,
				ConfigVariables:  variables,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config: released,
				Check:  cp.remove(k, &declares),
			},
		},
	})
}

func (cp controlPlane) remove(k keys, declares *bool) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()

		if *declares {
			if err := cp.undeclare(ctx); err != nil {
				return fmt.Errorf("could not delete the declaration of the workspace's runners: %w", err)
			}
			*declares = false
		}

		for _, each := range cp.created(k) {
			meta, err := each.find(ctx)
			if spotlight.IsNotFound(err) {
				continue
			}
			if err == nil {
				err = each.remove(ctx, meta)
			}
			if err != nil {
				return fmt.Errorf("could not delete the %s %s: %w", each.kind, each.address, err)
			}
		}

		return nil
	}
}
