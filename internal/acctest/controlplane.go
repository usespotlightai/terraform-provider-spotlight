package acctest

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
)

// WorkspaceEnv names the workspace the acceptance tests work in, by its
// slug.
const WorkspaceEnv = "SPOTLIGHT_ACC_WORKSPACE"

// The prefixes of everything the acceptance tests create, which the
// sweepers delete: KeyPrefix starts the keys and names of teams, runners,
// runner trusts and runner pools, and so the address of everything a team
// holds;
// SecretPrefix starts the names of secrets.
const (
	KeyPrefix    = "tf-acc-"
	SecretPrefix = "TF_ACC_"
)

// ProtoV6ProviderFactories serves the provider as Terraform would run it,
// configured from the environment.
func ProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"spotlight": providerserver.NewProtocol6WithError(provider.New("acc")()),
	}
}

// ControlPlane is a real control plane the acceptance tests work against,
// and the workspace they work in there.
type ControlPlane struct {
	Client    *client.Client
	Workspace client.Workspace
}

// PreCheck skips the test unless TF_ACC is set and the environment names a
// control plane, a token there and the workspace to work in, and answers
// that control plane.
func PreCheck(t *testing.T) ControlPlane {
	t.Helper()

	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skipf("acceptance tests run only with %s=1", resource.EnvTfAcc)
	}

	for _, env := range []string{provider.URLEnv, provider.TokenEnv, WorkspaceEnv} {
		if os.Getenv(env) == "" {
			t.Skipf("acceptance tests need %s, %s and %s: a dev control plane, a token there and the slug of its workspace", provider.URLEnv, provider.TokenEnv, WorkspaceEnv)
		}
	}

	cp, err := ControlPlaneAt(t.Context(), os.Getenv(WorkspaceEnv))
	if err != nil {
		t.Fatal(err)
	}

	return cp
}

// ControlPlaneAt connects to the control plane the environment names, to
// work in the workspace at slug.
func ControlPlaneAt(ctx context.Context, slug string) (ControlPlane, error) {
	api, err := client.New(client.Config{
		URL:             os.Getenv(provider.URLEnv),
		Token:           os.Getenv(provider.TokenEnv),
		ProviderVersion: "acc",
		Retries:         2,
		Backoff:         client.DefaultBackoff,
	})
	if err != nil {
		return ControlPlane{}, err
	}

	workspace, err := api.ImportWorkspace(ctx, slug)
	if err != nil {
		return ControlPlane{}, fmt.Errorf("the token cannot see the workspace %s: %w", slug, err)
	}

	return ControlPlane{Client: api, Workspace: workspace}, nil
}

// Keys are the keys and names of what one acceptance test creates, random
// so that tests can run side by side.
type Keys struct {
	Team, Line, Skill, Integration, Profile string
	Secret, AWSSecret                       string
	Runner, IssuedRunner, Trust             string
	Pool, PushPool                          string
}

// RandomKeys answers keys no other test uses.
func RandomKeys() Keys {
	suffix := strings.ToLower(acctest.RandStringFromCharSet(10, acctest.CharSetAlpha))
	upper := strings.ToUpper(suffix)

	return Keys{
		Team:         KeyPrefix + suffix,
		Line:         "delivery-" + suffix,
		Skill:        "grafana-" + suffix,
		Integration:  "tfacc" + suffix,
		Profile:      "careful-" + suffix,
		Secret:       SecretPrefix + upper,
		AWSSecret:    SecretPrefix + "AWS_" + upper,
		Runner:       KeyPrefix + "build-" + suffix,
		IssuedRunner: KeyPrefix + "issued-" + suffix,
		Trust:        KeyPrefix + "cluster-" + suffix,
		Pool:         KeyPrefix + "pool-" + suffix,
		PushPool:     KeyPrefix + "push-" + suffix,
	}
}

// RandomToken answers a random value long enough for a secret or a
// runner's token.
func RandomToken() string {
	return "tfacc" + acctest.RandStringFromCharSet(40, acctest.CharSetAlphaNum)
}

// RandomSigningSecret answers a random secret a push pool signs its
// provision calls with, as Standard Webhooks write one.
func RandomSigningSecret() string {
	return "whsec_" + base64.StdEncoding.EncodeToString([]byte(acctest.RandStringFromCharSet(32, acctest.CharSetAlphaNum)))
}

// AdoptedWorkspace is configuration that imports the workspace and
// protects it from destroy, with local.workspace_id naming it.
func AdoptedWorkspace(workspace client.Workspace) string {
	return fmt.Sprintf(`
import {
  to = spotlight_workspace.acc
  id = %[2]q
}

resource "spotlight_workspace" "acc" {
  name = %[1]q
  slug = %[2]q

  lifecycle {
    prevent_destroy = true
  }
}

locals {
  workspace_id = spotlight_workspace.acc.id
}
`, workspace.Name, workspace.Slug)
}

// ReleasedWorkspace is configuration that forgets the workspace
// AdoptedWorkspace imported without destroying it, with local.workspace_id
// still naming it.
func ReleasedWorkspace(workspace client.Workspace) string {
	return fmt.Sprintf(`
removed {
  from = spotlight_workspace.acc

  lifecycle {
    destroy = false
  }
}

locals {
  workspace_id = %q
}
`, workspace.ID)
}

// ImportID is the id a resource is imported by: its workspace and its
// address there.
func ImportID(workspaceID, address string) string {
	return workspaceID + "/" + address
}

// ExpectForgotten is a plan check that the plan forgets the resource at
// address without destroying it.
func ExpectForgotten(address string) plancheck.PlanCheck {
	return forgotten(address)
}

type forgotten string

func (address forgotten) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	for _, each := range req.Plan.ResourceChanges {
		if each.Address == string(address) {
			if !each.Change.Actions.Forget() {
				resp.Error = fmt.Errorf("%s is planned to %v, not to be forgotten", address, each.Change.Actions)
			}

			return
		}
	}

	resp.Error = fmt.Errorf("%s is not in the plan", address)
}

type held struct {
	kind, address string
	find          func(context.Context) (client.Meta, error)
	remove        func(context.Context, string, int64) error
}

func holding[T any](kind string, find func(context.Context, string, string) (T, error), metaOf func(T) client.Meta, remove func(context.Context, string, int64) error, workspaceID, address string) held {
	return held{
		kind:    kind,
		address: address,
		find: func(ctx context.Context) (client.Meta, error) {
			found, err := find(ctx, workspaceID, address)

			return metaOf(found), err
		},
		remove: remove,
	}
}

func (cp ControlPlane) created(k Keys) []held {
	api, id := cp.Client, cp.Workspace.ID
	inTeam := func(key string) string { return k.Team + "/" + key }

	return []held{
		holding("line", api.ImportLine, func(l client.Line) client.Meta { return l.Meta }, api.DeleteLine, id, inTeam(k.Line)),
		holding("skill", api.ImportSkill, func(s client.Skill) client.Meta { return s.Meta }, api.DeleteSkill, id, inTeam(k.Skill)),
		holding("integration", api.ImportIntegration, func(i client.Integration) client.Meta { return i.Meta }, api.DeleteIntegration, id, inTeam(k.Integration)),
		holding("harness profile", api.ImportHarnessProfile, func(p client.HarnessProfile) client.Meta { return p.Meta }, api.DeleteHarnessProfile, id, inTeam(k.Profile)),
		holding("team", api.ImportTeam, func(t client.Team) client.Meta { return t.Meta }, api.DeleteTeam, id, k.Team),
		holding("secret", api.ImportSecret, func(s client.Secret) client.Meta { return s.Meta }, api.DeleteSecret, id, k.Secret),
		holding("secret", api.ImportSecret, func(s client.Secret) client.Meta { return s.Meta }, api.DeleteSecret, id, k.AWSSecret),
		holding("runner", api.ImportRunner, func(r client.Runner) client.Meta { return r.Meta }, api.DeleteRunner, id, k.Runner),
		holding("runner", api.ImportRunner, func(r client.Runner) client.Meta { return r.Meta }, api.DeleteRunner, id, k.IssuedRunner),
		holding("runner trust", api.ImportRunnerTrust, func(t client.RunnerTrust) client.Meta { return t.Meta }, api.DeleteRunnerTrust, id, k.Trust),
		holding("runner pool", api.ImportRunnerPool, func(p client.RunnerPool) client.Meta { return p.Meta }, api.DeleteRunnerPool, id, k.Pool),
		holding("runner pool", api.ImportRunnerPool, func(p client.RunnerPool) client.Meta { return p.Meta }, api.DeleteRunnerPool, id, k.PushPool),
	}
}

// RemoveCreated deletes whatever the test with these keys created that is
// still there.
func (cp ControlPlane) RemoveCreated(ctx context.Context, k Keys) error {
	var failed []error

	for _, each := range cp.created(k) {
		meta, err := each.find(ctx)
		if client.IsNotFound(err) {
			continue
		}
		if err == nil {
			err = each.remove(ctx, meta.ID, meta.Version)
		}
		if err != nil {
			failed = append(failed, fmt.Errorf("the %s %s: %w", each.kind, each.address, err))
		}
	}

	return errors.Join(failed...)
}

// Sweep deletes whatever the test with these keys created once it ends,
// however it ends.
func (cp ControlPlane) Sweep(t *testing.T, k Keys) {
	t.Logf("working in workspace %s under team %s", cp.Workspace.Slug, k.Team)

	t.Cleanup(func() {
		if err := cp.RemoveCreated(context.Background(), k); err != nil {
			t.Errorf("left behind: %v", err)
		}
	})
}

// NothingLeftButTheWorkspace checks that destroying the configuration left
// the workspace as it was and removed everything the test created.
func (cp ControlPlane) NothingLeftButTheWorkspace(k Keys) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()

		workspace, err := cp.Client.ImportWorkspace(ctx, cp.Workspace.Slug)
		if err != nil {
			return fmt.Errorf("the workspace %s did not survive: %w", cp.Workspace.Slug, err)
		}

		if workspace.ID != cp.Workspace.ID || workspace.Version != cp.Workspace.Version || workspace.Name != cp.Workspace.Name {
			return fmt.Errorf("the workspace changed: %+v, was %+v", workspace, cp.Workspace)
		}

		var left []error
		for _, each := range cp.created(k) {
			if _, err := each.find(ctx); !client.IsNotFound(err) {
				left = append(left, fmt.Errorf("the %s %s is still there (%v)", each.kind, each.address, err))
			}
		}

		return errors.Join(left...)
	}
}

// DeclaredRunners reads the workspace's declaration of its runners.
func (cp ControlPlane) DeclaredRunners(ctx context.Context) (client.WorkspaceRunners, error) {
	return cp.Client.ImportWorkspaceRunners(ctx, cp.Workspace.ID, cp.Workspace.Slug)
}

// Undeclare deletes the workspace's declaration of its runners, if it has
// one.
func (cp ControlPlane) Undeclare(ctx context.Context) error {
	declared, err := cp.DeclaredRunners(ctx)
	if client.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	return cp.Client.DeleteWorkspaceRunners(ctx, declared.ID, declared.Version)
}
