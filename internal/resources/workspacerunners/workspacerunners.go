// Package workspacerunners is the spotlight_workspace_runners resource.
package workspacerunners

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const kind = "declaration"

type model struct {
	ID          types.String `tfsdk:"id"`
	Version     types.Int64  `tfsdk:"version"`
	Address     types.String `tfsdk:"address"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
	Runners     types.Set    `tfsdk:"runners"`
	Trusts      types.Set    `tfsdk:"trusts"`
	Drift       types.Set    `tfsdk:"drift"`
}

// New returns the spotlight_workspace_runners resource.
func New() resource.Resource {
	return resourcekit.NewManaged(resourcekit.Spec[spotlight.WorkspaceRunnersInput, spotlight.WorkspaceRunners, model]{
		Kind:        "workspace_runners",
		Schema:      schemaOf,
		AddressedAs: "acme",
		Needs:       spotlight.ScopeRunners,
		WarnOnRead:  true,
		Resources: func(api spotlight.API) spotlight.Resources[spotlight.WorkspaceRunnersInput, spotlight.WorkspaceRunners] {
			return api.WorkspaceRunners
		},
		Owner:    func(m model) string { return m.WorkspaceID.ValueString() },
		Input:    inputOf,
		Stored:   stored,
		Meta:     func(w spotlight.WorkspaceRunners) spotlight.Meta { return w.Meta },
		Settable: func(w spotlight.WorkspaceRunners) any { return w.WorkspaceRunnersInput },
		Identity: func(m model) (string, int64) { return m.ID.ValueString(), m.Version.ValueInt64() },
	})
}

var schemaOf = schema.Schema{
	MarkdownDescription: "The runners a workspace means to have, declared once for the workspace. A runner enrolled all the same is drift: " +
		"it still runs, and every plan warns about it until it is declared here or deleted. So does a name here that no runner or trust answers to. " +
		"Writing the declaration needs a token granted the `runners` scope. Import it as `<workspace id>/<workspace slug>`.",
	Attributes: map[string]schema.Attribute{
		"id":           resourcekit.IDAttribute(kind),
		"version":      resourcekit.VersionAttribute(kind),
		"address":      resourcekit.AddressAttribute("acme"),
		"workspace_id": resourcekit.OwnerAttribute("workspace", kind),
		"runners":      resourcekit.StringSetAttribute("The runners the workspace means to have, by name."),
		"trusts":       resourcekit.StringSetAttribute("The runner trusts, by key, whose runners the workspace means to have."),
		"drift": schema.SetAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "The runners enrolled all the same, outside what is declared.",
		},
	},
}

func inputOf(plan model) (spotlight.WorkspaceRunnersInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	return spotlight.WorkspaceRunnersInput{
		Runners: resourcekit.StringsIn(plan.Runners, &diags),
		Trusts:  resourcekit.StringsIn(plan.Trusts, &diags),
	}, diags
}

func stored(answer spotlight.WorkspaceRunners, kept *model) (model, diag.Diagnostics) {
	held := model{
		ID:          types.StringValue(answer.ID),
		Version:     types.Int64Value(answer.Version),
		Address:     types.StringValue(answer.Address),
		WorkspaceID: types.StringValue(answer.WorkspaceID),
		Runners:     resourcekit.StringSet(answer.Runners),
		Trusts:      resourcekit.StringSet(answer.Trusts),
		Drift:       resourcekit.StringSet(answer.Drift),
	}

	if kept != nil {
		held.Runners = kept.Runners
		held.Trusts = kept.Trusts
	}

	return held, nil
}
