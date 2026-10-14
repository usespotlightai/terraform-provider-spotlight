package acctest

import (
	"testing"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
)

// FakeClient calls fake. A test uses it to prepare the resources that
// Terraform finds there.
func FakeClient(t *testing.T, fake *fakeapi.Server) *client.Client {
	t.Helper()

	api, err := client.New(client.Config{URL: fake.URL, Token: fakeapi.Token, ProviderVersion: Version})
	if err != nil {
		t.Fatal(err)
	}

	return api
}

// Seeded contains the resources that Seed made without Terraform:
//   - The workspace acme with the teams core and platform.
//   - A different workspace, globex, with its own team core.
type Seeded struct {
	Client    *client.Client
	Workspace client.Workspace
	Core      client.Team
	Platform  client.Team
	Other     client.Workspace
	OtherCore client.Team
}

// SeededConfig is configuration that reads the seeded workspace. It is for
// the step that prepares Terraform before terraform query.
const SeededConfig = `
data "spotlight_workspace" "acme" {
  slug = "acme"
}
`

// Seed makes Seeded in fake.
func Seed(t *testing.T, fake *fakeapi.Server) Seeded {
	t.Helper()

	ctx := t.Context()
	api := FakeClient(t, fake)

	workspace, err := api.CreateWorkspace(ctx, client.WorkspaceInput{Name: "Acme", Slug: "acme"})
	Must(t, err)
	core, err := api.CreateTeam(ctx, workspace.ID, client.TeamInput{Key: "core", Name: "Core"})
	Must(t, err)
	platform, err := api.CreateTeam(ctx, workspace.ID, client.TeamInput{Key: "platform", Name: "Platform"})
	Must(t, err)
	other, err := api.CreateWorkspace(ctx, client.WorkspaceInput{Name: "Globex", Slug: "globex"})
	Must(t, err)
	otherCore, err := api.CreateTeam(ctx, other.ID, client.TeamInput{Key: "core", Name: "Core"})
	Must(t, err)

	return Seeded{Client: api, Workspace: workspace, Core: core, Platform: platform, Other: other, OtherCore: otherCore}
}

// Must fails the test if a necessary call failed.
func Must(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}
