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

// WorkspaceEnv identifies the workspace of the acceptance tests by its slug.
const WorkspaceEnv = "SPOTLIGHT_ACC_WORKSPACE"

// The prefixes of all the resources that the acceptance tests create. The
// sweepers delete these resources.
//   - KeyPrefix starts the keys and names of teams, runners, runner trusts,
//     runner pools, egresses and OIDC trusts. An address in a team starts
//     with the team key, so KeyPrefix also starts each of these addresses.
//   - SecretPrefix starts the names of secrets.
const (
	KeyPrefix    = "tf-acc-"
	SecretPrefix = "TF_ACC_"
)

// ProtoV6ProviderFactories serves the provider as Terraform runs it,
// configured from the environment.
func ProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"spotlight": providerserver.NewProtocol6WithError(provider.New("acc")()),
	}
}

// ControlPlane is a real control plane for the acceptance tests, and the
// workspace of the tests on that control plane.
type ControlPlane struct {
	Client    *client.Client
	Workspace client.Workspace
}

// PreCheck skips the test if TF_ACC is not set, or if the environment does
// not identify a control plane, a token for it and a workspace. If the test
// continues, PreCheck returns that control plane. The token must have configure, secrets,
// runners and trusts, because the tests write all these resources.
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

// ControlPlaneAt connects to the control plane that the environment
// identifies, for the workspace at slug.
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

// Keys are the keys and names of the resources that one acceptance test
// creates. They are random, so that tests can run at the same time.
type Keys struct {
	Team, Line, Skill, Integration, Profile string
	Hook                                    string
	Secret, AWSSecret                       string
	Trust, Pool, PushPool                   string
	Egress                                  string
	OidcTrust                               string
}

// RandomKeys returns keys that no other test uses.
func RandomKeys() Keys {
	suffix := strings.ToLower(acctest.RandStringFromCharSet(10, acctest.CharSetAlpha))
	upper := strings.ToUpper(suffix)

	return Keys{
		Team:        KeyPrefix + suffix,
		Line:        "delivery-" + suffix,
		Skill:       "grafana-" + suffix,
		Integration: "tfacc" + suffix,
		Profile:     "careful-" + suffix,
		Hook:        "audit-" + suffix,
		Secret:      SecretPrefix + upper,
		AWSSecret:   SecretPrefix + "AWS_" + upper,
		Trust:       KeyPrefix + "cluster-" + suffix,
		Pool:        KeyPrefix + "pool-" + suffix,
		PushPool:    KeyPrefix + "push-" + suffix,
		Egress:      KeyPrefix + "egress-" + suffix,
		OidcTrust:   KeyPrefix + "github-" + suffix,
	}
}

// RandomToken returns a random value that is long enough for a secret, for
// the token of a pull pool or for the token of an egress.
func RandomToken() string {
	return "tfacc" + acctest.RandStringFromCharSet(40, acctest.CharSetAlphaNum)
}

// RandomSigningSecret returns a random secret in the Standard Webhooks
// format. A push pool signs its provision calls with it.
func RandomSigningSecret() string {
	return "whsec_" + base64.StdEncoding.EncodeToString([]byte(acctest.RandStringFromCharSet(32, acctest.CharSetAlphaNum)))
}

// AdoptedWorkspace is configuration that imports the workspace and
// protects it from destroy. local.workspace_id identifies the workspace.
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

// ReleasedWorkspace is configuration that removes the workspace that
// AdoptedWorkspace imported from the state, but does not destroy it.
// local.workspace_id continues to identify the workspace.
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

// ImportID is the import id of a resource. It contains the workspace of the
// resource and its address in that workspace.
func ImportID(workspaceID, address string) string {
	return workspaceID + "/" + address
}

// ExpectForgotten is a plan check that the plan removes the resource at
// address from the state but does not destroy it.
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

type sweepable struct {
	kind, address string
	find          func(context.Context) (client.Meta, error)
	remove        func(context.Context, string, int64) error
}

func sweepableOf[T any](kind string, find func(context.Context, string, string) (T, error), metaOf func(T) client.Meta, remove func(context.Context, string, int64) error, workspaceID, address string) sweepable {
	return sweepable{
		kind:    kind,
		address: address,
		find: func(ctx context.Context) (client.Meta, error) {
			found, err := find(ctx, workspaceID, address)

			return metaOf(found), err
		},
		remove: remove,
	}
}

func (cp ControlPlane) created(k Keys) []sweepable {
	api, id := cp.Client, cp.Workspace.ID
	inTeam := func(key string) string { return k.Team + "/" + key }

	return []sweepable{
		sweepableOf("hook", api.ImportHook, func(h client.Hook) client.Meta { return h.Meta }, api.DeleteHook, id, inTeam(k.Hook)),
		sweepableOf("line", api.ImportLine, func(l client.Line) client.Meta { return l.Meta }, api.DeleteLine, id, inTeam(k.Line)),
		sweepableOf("skill", api.ImportSkill, func(s client.Skill) client.Meta { return s.Meta }, api.DeleteSkill, id, inTeam(k.Skill)),
		sweepableOf("integration", api.ImportIntegration, func(i client.Integration) client.Meta { return i.Meta }, api.DeleteIntegration, id, inTeam(k.Integration)),
		sweepableOf("harness profile", api.ImportHarnessProfile, func(p client.HarnessProfile) client.Meta { return p.Meta }, api.DeleteHarnessProfile, id, inTeam(k.Profile)),
		sweepableOf("team", api.ImportTeam, func(t client.Team) client.Meta { return t.Meta }, api.DeleteTeam, id, k.Team),
		sweepableOf("secret", api.ImportSecret, func(s client.Secret) client.Meta { return s.Meta }, api.DeleteSecret, id, k.Secret),
		sweepableOf("secret", api.ImportSecret, func(s client.Secret) client.Meta { return s.Meta }, api.DeleteSecret, id, k.AWSSecret),
		sweepableOf("runner trust", api.ImportRunnerTrust, func(t client.RunnerTrust) client.Meta { return t.Meta }, api.DeleteRunnerTrust, id, k.Trust),
		sweepableOf("runner pool", api.ImportRunnerPool, func(p client.RunnerPool) client.Meta { return p.Meta }, api.DeleteRunnerPool, id, k.Pool),
		sweepableOf("runner pool", api.ImportRunnerPool, func(p client.RunnerPool) client.Meta { return p.Meta }, api.DeleteRunnerPool, id, k.PushPool),
		sweepableOf("egress", api.ImportEgress, func(e client.Egress) client.Meta { return e.Meta }, api.DeleteEgress, id, k.Egress),
		sweepableOf("OIDC trust", api.ImportOidcTrust, func(t client.OidcTrust) client.Meta { return t.Meta }, api.DeleteOidcTrust, id, k.OidcTrust),
	}
}

// RemoveCreated deletes all the resources that the test with these keys
// created and that are still there.
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

// Sweep deletes all the resources that the test with these keys created,
// after the test ends. It does this also if the test fails.
func (cp ControlPlane) Sweep(t *testing.T, k Keys) {
	t.Logf("working in workspace %s under team %s", cp.Workspace.Slug, k.Team)

	t.Cleanup(func() {
		if err := cp.RemoveCreated(context.Background(), k); err != nil {
			t.Errorf("left behind: %v", err)
		}
	})
}

// NothingLeftButTheWorkspace checks that the destroy of the configuration
// did not change the workspace and removed all the resources that the test
// created.
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

// DeclaredRunners reads the declaration of the runners of the workspace.
func (cp ControlPlane) DeclaredRunners(ctx context.Context) (client.WorkspaceRunners, error) {
	return cp.Client.ImportWorkspaceRunners(ctx, cp.Workspace.ID, cp.Workspace.Slug)
}

// Undeclare deletes the declaration of the runners of the workspace, if it
// has one.
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
