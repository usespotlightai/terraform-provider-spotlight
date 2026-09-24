// Package team is the spotlight_team resource.
package team

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const kind = "team"

type model struct {
	ID          types.String `tfsdk:"id"`
	Version     types.Int64  `tfsdk:"version"`
	Address     types.String `tfsdk:"address"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
	Key         types.String `tfsdk:"key"`
	Name        types.String `tfsdk:"name"`
}

// New returns the spotlight_team resource.
func New() resource.Resource {
	return resourcekit.NewManaged(resourcekit.Spec[spotlight.TeamInput, spotlight.Team, model]{
		Kind:        kind,
		Schema:      schemaOf,
		AddressedAs: "core",
		Resources:   func(api spotlight.API) spotlight.Resources[spotlight.TeamInput, spotlight.Team] { return api.Teams },
		Owner:       func(m model) string { return m.WorkspaceID.ValueString() },
		Input:       inputOf,
		Stored:      stored,
		Meta:        func(t spotlight.Team) spotlight.Meta { return t.Meta },
		Settable:    func(t spotlight.Team) any { return spotlight.TeamInput{Key: t.Key, Name: t.Name} },
		Identity:    func(m model) (string, int64) { return m.ID.ValueString(), m.Version.ValueInt64() },
	})
}

var schemaOf = schema.Schema{
	MarkdownDescription: "A team in a Spotlight workspace, holding its lines, skills and integrations. Import it as `<workspace id>/<key>`.",
	Attributes: map[string]schema.Attribute{
		"id":           resourcekit.IDAttribute(kind),
		"version":      resourcekit.VersionAttribute(kind),
		"address":      resourcekit.AddressAttribute("core"),
		"workspace_id": resourcekit.OwnerAttribute("workspace", kind),
		"key":          resourcekit.FixedAttribute("Names the team in the addresses of everything it holds.", kind),
		"name": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "What the team is called.",
		},
	},
}

func inputOf(plan model) (spotlight.TeamInput, diag.Diagnostics) {
	return spotlight.TeamInput{Key: plan.Key.ValueString(), Name: plan.Name.ValueString()}, nil
}

func stored(answer spotlight.Team, kept *model) (model, diag.Diagnostics) {
	held := model{
		ID:          types.StringValue(answer.ID),
		Version:     types.Int64Value(answer.Version),
		Address:     types.StringValue(answer.Address),
		WorkspaceID: types.StringValue(answer.WorkspaceID),
		Key:         types.StringValue(answer.Key),
		Name:        types.StringValue(answer.Name),
	}

	if kept != nil {
		held.Key = kept.Key
		held.Name = kept.Name
	}

	return held, nil
}
