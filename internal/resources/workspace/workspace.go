// Package workspace is the spotlight_workspace resource.
package workspace

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const kind = "workspace"

type model struct {
	ID      types.String `tfsdk:"id"`
	Version types.Int64  `tfsdk:"version"`
	Address types.String `tfsdk:"address"`
	Name    types.String `tfsdk:"name"`
	Slug    types.String `tfsdk:"slug"`
}

// New returns the spotlight_workspace resource.
func New() resource.Resource {
	return resourcekit.NewManaged(resourcekit.Spec[spotlight.WorkspaceInput, spotlight.Workspace, model]{
		Kind:                   kind,
		Schema:                 schemaOf,
		AddressedAs:            "acme",
		ImportedByAddressAlone: true,
		Resources: func(api spotlight.API) spotlight.Resources[spotlight.WorkspaceInput, spotlight.Workspace] {
			return api.Workspaces
		},
		Input:    inputOf,
		Stored:   stored,
		Meta:     func(w spotlight.Workspace) spotlight.Meta { return w.Meta },
		Settable: func(w spotlight.Workspace) any { return spotlight.WorkspaceInput{Name: w.Name, Slug: w.Slug} },
		Identity: func(m model) (string, int64) { return m.ID.ValueString(), m.Version.ValueInt64() },
	})
}

var schemaOf = schema.Schema{
	MarkdownDescription: "A Spotlight workspace, owned by whoever creates it. Import it by its slug.",
	Attributes: map[string]schema.Attribute{
		"id":      resourcekit.IDAttribute(kind),
		"version": resourcekit.VersionAttribute(kind),
		"address": resourcekit.AddressAttribute("acme"),
		"name": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "What the workspace is called.",
		},
		"slug": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Where the workspace lives in the web app. It is fixed for the workspace's life, so changing it replaces the workspace.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
	},
}

func inputOf(plan model) (spotlight.WorkspaceInput, diag.Diagnostics) {
	return spotlight.WorkspaceInput{Name: plan.Name.ValueString(), Slug: plan.Slug.ValueString()}, nil
}

func stored(answer spotlight.Workspace, kept *model) (model, diag.Diagnostics) {
	held := model{
		ID:      types.StringValue(answer.ID),
		Version: types.Int64Value(answer.Version),
		Address: types.StringValue(answer.Address),
		Name:    types.StringValue(answer.Name),
		Slug:    types.StringValue(answer.Slug),
	}

	if kept != nil {
		held.Name = kept.Name
		held.Slug = kept.Slug
	}

	return held, nil
}
