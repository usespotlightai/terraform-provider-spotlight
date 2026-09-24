// Package integration is the spotlight_integration resource.
package integration

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/dynamic"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/expressions"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fieldpath"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const (
	kind = "integration"
	host = "integration"
)

type model struct {
	ID         types.String  `tfsdk:"id"`
	Version    types.Int64   `tfsdk:"version"`
	Address    types.String  `tfsdk:"address"`
	TeamID     types.String  `tfsdk:"team_id"`
	Source     types.String  `tfsdk:"source"`
	Definition types.Dynamic `tfsdk:"definition"`
}

type managed = resourcekit.Managed[spotlight.Definition, spotlight.Integration, model]

// Resource is the spotlight_integration resource.
type Resource struct {
	*managed
}

var (
	_ resource.ResourceWithValidateConfig = Resource{}
	_ resource.ResourceWithModifyPlan     = Resource{}
	_ resource.ResourceWithImportState    = Resource{}
)

// New returns the spotlight_integration resource.
func New() resource.Resource {
	return Resource{resourcekit.NewManaged(resourcekit.Spec[spotlight.Definition, spotlight.Integration, model]{
		Kind:        kind,
		Schema:      schemaOf,
		Under:       fieldpath.Path{fieldpath.Key(expressions.Attribute)},
		AddressedAs: "core/linear",
		Resources: func(api spotlight.API) spotlight.Resources[spotlight.Definition, spotlight.Integration] {
			return api.Integrations
		},
		Owner:    func(m model) string { return m.TeamID.ValueString() },
		Input:    inputOf,
		Stored:   stored,
		Meta:     func(i spotlight.Integration) spotlight.Meta { return i.Meta },
		Settable: func(i spotlight.Integration) any { return i.Definition },
		Identity: func(m model) (string, int64) { return m.ID.ValueString(), m.Version.ValueInt64() },
	})}
}

var schemaOf = schema.Schema{
	MarkdownDescription: "An integration a team has defined: how Spotlight hears from a host, and the actions and tools it offers against it. " +
		"The definition is held whole, exactly as the integration's definition is written, with its keys verbatim; write each expression as `$${{ ... }}` so Terraform leaves it alone. " +
		"`terraform plan` type-checks every expression in it where it is written. Import it as `<workspace id>/<team key>/<source>`.",
	Attributes: map[string]schema.Attribute{
		"id":      resourcekit.IDAttribute(kind),
		"version": resourcekit.VersionAttribute(kind),
		"address": resourcekit.AddressAttribute("core/linear"),
		"team_id": resourcekit.OwnerAttribute("team", kind),
		"source": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The host the integration defines, from `definition.integration`. It is fixed for the integration's life, so changing it replaces the integration.",
		},
		expressions.Attribute: schema.DynamicAttribute{
			Required:            true,
			MarkdownDescription: "The integration's whole definition, as an object. Spotlight fills in each field it leaves out when it reads it back.",
		},
	},
}

func inputOf(plan model) (spotlight.Definition, diag.Diagnostics) {
	var diags diag.Diagnostics

	held, err := dynamic.ToJSON(plan.Definition)
	if err != nil {
		diags.AddAttributeError(path.Root(expressions.Attribute), "Definition not known", err.Error())

		return nil, diags
	}

	definition, ok := held.(map[string]any)
	if !ok {
		diags.AddAttributeError(path.Root(expressions.Attribute), "Definition is not an object", "Write the definition as an object, as in { integration = \"linear\", ... }.")

		return nil, diags
	}

	return definition, diags
}

func stored(answer spotlight.Integration, kept *model) (model, diag.Diagnostics) {
	held := model{
		ID:         types.StringValue(answer.ID),
		Version:    types.Int64Value(answer.Version),
		Address:    types.StringValue(answer.Address),
		TeamID:     types.StringValue(answer.TeamID),
		Source:     types.StringValue(answer.Source),
		Definition: dynamic.FromJSON(answer.Definition),
	}

	if kept != nil {
		held.Definition = dynamic.Resolve(kept.Definition, answer.Definition)
	}

	return held, nil
}

// ValidateConfig type-checks the definition's expressions once the provider
// is configured, as it is during a plan.
func (r Resource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	clients := r.Clients()
	if clients == nil || clients.Expressions == nil {
		return
	}

	var definition types.Dynamic
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(expressions.Attribute), &definition)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(clients.Expressions.Check(ctx, definition)...)
}

// ModifyPlan plans the source from the definition, and replaces the
// integration when the definition names another host.
func (r Resource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var definition types.Dynamic
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(expressions.Attribute), &definition)...)
	if resp.Diagnostics.HasError() {
		return
	}

	source, known := hostIn(definition)
	if !known {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("source"), source)...)

	if req.State.Raw.IsNull() {
		return
	}

	var prior types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("source"), &prior)...)

	if !prior.IsNull() && prior.ValueString() != source {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root(expressions.Attribute).AtName(host))
	}
}

func hostIn(definition types.Dynamic) (string, bool) {
	if definition.IsNull() || definition.IsUnknown() || definition.IsUnderlyingValueUnknown() {
		return "", false
	}

	object, ok := definition.UnderlyingValue().(basetypes.ObjectValue)
	if !ok {
		return "", false
	}

	named, ok := object.Attributes()[host].(basetypes.StringValue)
	if !ok || named.IsNull() || named.IsUnknown() {
		return "", false
	}

	return named.ValueString(), true
}
