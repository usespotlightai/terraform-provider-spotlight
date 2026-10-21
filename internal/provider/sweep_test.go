package provider_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
)

func TestMain(m *testing.M) {
	resource.TestMain(m)
}

var sweepers = []*resource.Sweeper{
	{
		Name: "spotlight_line",
		F:    sweeper((*client.Client).ListLines, (*client.Client).DeleteLine, func(l client.Line) client.Meta { return l.Meta }, acctest.KeyPrefix),
	},
	{
		Name:         "spotlight_skill",
		Dependencies: []string{"spotlight_line"},
		F:            sweeper((*client.Client).ListSkills, (*client.Client).DeleteSkill, func(s client.Skill) client.Meta { return s.Meta }, acctest.KeyPrefix),
	},
	{
		Name: "spotlight_hook",
		F:    sweeper((*client.Client).ListHooks, (*client.Client).DeleteHook, func(h client.Hook) client.Meta { return h.Meta }, acctest.KeyPrefix),
	},
	{
		Name:         "spotlight_integration",
		Dependencies: []string{"spotlight_line", "spotlight_hook"},
		F:            sweeper((*client.Client).ListIntegrations, (*client.Client).DeleteIntegration, func(i client.Integration) client.Meta { return i.Meta }, acctest.KeyPrefix),
	},
	{
		Name:         "spotlight_harness_profile",
		Dependencies: []string{"spotlight_line"},
		F:            sweeper((*client.Client).ListHarnessProfiles, (*client.Client).DeleteHarnessProfile, func(p client.HarnessProfile) client.Meta { return p.Meta }, acctest.KeyPrefix),
	},
	{
		Name:         "spotlight_team",
		Dependencies: []string{"spotlight_line", "spotlight_skill", "spotlight_integration", "spotlight_harness_profile", "spotlight_hook"},
		F:            sweeper((*client.Client).ListTeams, (*client.Client).DeleteTeam, func(t client.Team) client.Meta { return t.Meta }, acctest.KeyPrefix),
	},
	{
		Name:         "spotlight_secret",
		Dependencies: []string{"spotlight_integration"},
		F:            sweeper((*client.Client).ListSecrets, (*client.Client).DeleteSecret, func(s client.Secret) client.Meta { return s.Meta }, acctest.SecretPrefix),
	},
	{
		Name: "spotlight_runner_trust",
		F:    sweeper((*client.Client).ListRunnerTrusts, (*client.Client).DeleteRunnerTrust, func(t client.RunnerTrust) client.Meta { return t.Meta }, acctest.KeyPrefix),
	},
	{
		Name: "spotlight_runner_pool",
		F:    sweeper((*client.Client).ListRunnerPools, (*client.Client).DeleteRunnerPool, func(p client.RunnerPool) client.Meta { return p.Meta }, acctest.KeyPrefix),
	},
	{
		Name:         "spotlight_egress",
		Dependencies: []string{"spotlight_runner_pool"},
		F:            sweeper((*client.Client).ListEgresses, (*client.Client).DeleteEgress, func(e client.Egress) client.Meta { return e.Meta }, acctest.KeyPrefix),
	},
	{
		Name: "spotlight_oidc_trust",
		F: sweeper((*client.Client).ListOidcTrusts, (*client.Client).DeleteOidcTrust,
			func(t client.OidcTrust) client.Meta { return t.Meta }, acctest.KeyPrefix),
	},
}

func init() {
	for _, each := range sweepers {
		resource.AddTestSweepers(each.Name, each)
	}
}

func sweeper[T any](
	list func(*client.Client, context.Context, string) ([]T, error),
	remove func(*client.Client, context.Context, string, int64) error,
	metaOf func(T) client.Meta,
	prefix string,
) func(string) error {
	return func(region string) error {
		ctx := context.Background()

		if region == "" {
			region = os.Getenv(acctest.WorkspaceEnv)
		}

		cp, err := acctest.ControlPlaneAt(ctx, region)
		if err != nil {
			return err
		}

		listed, err := list(cp.Client, ctx, cp.Workspace.ID)
		if err != nil {
			return err
		}

		var failed []error
		for _, each := range listed {
			meta := metaOf(each)
			if !strings.HasPrefix(meta.Address, prefix) {
				continue
			}

			if err := remove(cp.Client, ctx, meta.ID, meta.Version); err != nil && !client.IsNotFound(err) {
				failed = append(failed, fmt.Errorf("could not sweep %s: %w", meta.Address, err))
			}
		}

		return errors.Join(failed...)
	}
}

func TestSweepersDeleteOnlyWhatAcceptanceTestsCreate(t *testing.T) {
	fake := fakeapi.New(t)
	seeded := acctest.Seed(t, fake)
	api, ctx := seeded.Client, t.Context()
	k := acctest.RandomKeys()

	t.Setenv(provider.URLEnv, fake.URL)
	t.Setenv(provider.TokenEnv, fakeapi.Token)
	t.Setenv(acctest.WorkspaceEnv, seeded.Workspace.Slug)

	swept, err := api.CreateTeam(ctx, seeded.Workspace.ID, client.TeamInput{Key: k.Team, Name: "Terraform acceptance"})
	acctest.Must(t, err)

	for _, team := range []client.Team{swept, seeded.Core} {
		_, err = api.CreateLine(ctx, team.ID, client.Definition{
			"key": "delivery", "name": "Delivery",
			"stations": []any{map[string]any{"key": "backlog", "name": "Backlog", "kind": "backlog"}},
		})
		acctest.Must(t, err)
		_, err = api.CreateSkill(ctx, team.ID, client.SkillInput{Key: "grafana", Files: map[string]string{"SKILL.md": "---\nname: grafana\n---\n"}})
		acctest.Must(t, err)
		_, err = api.CreateIntegration(ctx, team.ID, client.Definition{"integration": "linear"})
		acctest.Must(t, err)
		_, err = api.CreateHarnessProfile(ctx, team.ID, client.HarnessProfileInput{Key: "careful"})
		acctest.Must(t, err)
		_, err = api.CreateHook(ctx, team.ID, client.HookInput{
			Key: "audit", On: "run.ended",
			Steps:   []any{map[string]any{"name": "audit", "request": map[string]any{"url": "https://audit.acme.dev"}}},
			Signing: &client.HookSigning{OIDC: &client.HookOIDC{Audience: "https://audit.acme.dev"}},
		})
		acctest.Must(t, err)
	}

	for _, name := range []string{k.Secret, k.AWSSecret, "LINEAR_API_KEY"} {
		_, err = api.CreateSecret(ctx, seeded.Workspace.ID, client.SecretInput{
			Name: name, Source: client.SecretSource{Kind: client.SourceAWSSecretsManager, SecretID: "acme/" + name}, Bindings: []client.SecretBinding{},
		})
		acctest.Must(t, err)
	}

	for _, key := range []string{k.Trust, "production-cluster"} {
		_, err = api.CreateRunnerTrust(ctx, seeded.Workspace.ID, client.RunnerTrustInput{
			Key: key, Issuer: "https://oidc.example.com", Subject: "system:serviceaccount:spotlight:runner", Audience: "spotlight", Labels: []string{},
		})
		acctest.Must(t, err)
	}

	for _, key := range []string{k.Pool, "ephemeral"} {
		_, err = api.CreateRunnerPool(ctx, seeded.Workspace.ID, client.RunnerPoolInput{Key: key, Labels: []string{}})
		acctest.Must(t, err)
	}

	for _, key := range []string{k.OidcTrust, "github-main"} {
		_, err = api.CreateOidcTrust(ctx, seeded.Workspace.ID, client.OidcTrustInput{
			Key: key, Issuer: "https://token.actions.githubusercontent.com", Audience: "spotlight", Subject: "repo:acme/platform:*",
			Claims: map[string]string{}, Scopes: []string{"plan"}, LifetimeMinutes: 15,
		})
		acctest.Must(t, err)
	}

	var egresses []client.Egress
	for _, key := range []string{k.Egress, "shared"} {
		egress, err := api.CreateEgress(ctx, seeded.Workspace.ID, client.EgressInput{Key: key, URL: "https://egress.example.com", Token: acctest.RandomToken()})
		acctest.Must(t, err)
		egresses = append(egresses, egress)
	}

	_, err = api.CreateRunnerPool(ctx, seeded.Workspace.ID, client.RunnerPoolInput{
		Key: k.PushPool, Labels: []string{},
		Provision: &client.PoolProvision{URL: "https://provision.example.com/runs", Secret: acctest.RandomSigningSecret(), Egress: &egresses[0].ID},
	})
	acctest.Must(t, err)

	for _, each := range sweepers {
		if err := each.F(""); err != nil {
			t.Fatalf("the %s sweeper failed: %v", each.Name, err)
		}
	}

	for kind, want := range map[string]int{
		"team": 3, "line": 1, "skill": 1, "integration": 1, "harness-profile": 1, "hook": 1,
		"secret": 1, "runner-trust": 1, "runner-pool": 1, "egress": 1, "oidc-trust": 1, "workspace": 2,
	} {
		if got := fake.Count(kind); got != want {
			t.Errorf("%d of %s left after sweeping, want %d", got, kind, want)
		}
	}

	if left := fake.IDOf("team", k.Team); left != "" {
		t.Errorf("the team %s was not swept", k.Team)
	}
}
