// Package secret is the spotlight_secret resource.
package secret

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/writeonly"
)

const kind = "secret"

type model struct {
	ID           types.String `tfsdk:"id"`
	Version      types.Int64  `tfsdk:"version"`
	Address      types.String `tfsdk:"address"`
	WorkspaceID  types.String `tfsdk:"workspace_id"`
	Name         types.String `tfsdk:"name"`
	Source       source       `tfsdk:"source"`
	Bindings     []binding    `tfsdk:"bindings"`
	Value        types.String `tfsdk:"value_wo"`
	ValueVersion types.Int64  `tfsdk:"value_wo_version"`
	RotatedAt    types.String `tfsdk:"rotated_at"`
}

type source struct {
	Kind     types.String `tfsdk:"kind"`
	SecretID types.String `tfsdk:"secret_id"`
	Field    types.String `tfsdk:"field"`
}

type binding struct {
	Host   types.String `tfsdk:"host"`
	Header types.String `tfsdk:"header"`
	Prefix types.String `tfsdk:"prefix"`
}

var (
	sourceType = map[string]attr.Type{
		"kind":      types.StringType,
		"secret_id": types.StringType,
		"field":     types.StringType,
	}
	bindingType = types.ObjectType{AttrTypes: map[string]attr.Type{
		"host":   types.StringType,
		"header": types.StringType,
		"prefix": types.StringType,
	}}
)

type managed = resourcekit.Managed[spotlight.SecretInput, spotlight.Secret, model]

// Resource is the spotlight_secret resource.
type Resource struct {
	*managed
}

var (
	_ resource.ResourceWithValidateConfig = Resource{}
	_ resource.ResourceWithImportState    = Resource{}
)

// New returns the spotlight_secret resource.
func New() resource.Resource {
	return Resource{resourcekit.NewManaged(resourcekit.Spec[spotlight.SecretInput, spotlight.Secret, model]{
		Kind:        kind,
		Schema:      schemaOf(),
		AddressedAs: "LINEAR_API_KEY",
		Needs:       spotlight.ScopeSecrets,
		Resources: func(api spotlight.API) spotlight.Resources[spotlight.SecretInput, spotlight.Secret] {
			return api.Secrets
		},
		Owner:    func(m model) string { return m.WorkspaceID.ValueString() },
		Input:    inputOf,
		Sent:     sent,
		Stored:   stored,
		Meta:     func(s spotlight.Secret) spotlight.Meta { return s.Meta },
		Settable: settable,
		Identity: func(m model) (string, int64) { return m.ID.ValueString(), m.Version.ValueInt64() },
	})}
}

// ValidateConfig refuses a value for a secret read from AWS Secrets
// Manager, which keeps it there.
func (r Resource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var kept, value types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("source").AtName("kind"), &kept)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(writeonly.Value.Name), &value)...)

	if kept.ValueString() == spotlight.SourceAWSSecretsManager && !value.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root(writeonly.Value.Name), "A secret read from AWS takes no value",
			"value_wo: is kept only by Spotlight; a secret read from AWS Secrets Manager is set there. Leave value_wo out, or keep the secret in Spotlight with source.kind = \"built-in\".")
	}
}

func schemaOf() schema.Schema {
	attributes := writeonly.Value.Attributes("The value Spotlight keeps, usually read from a sensitive variable. " +
		"A secret read from AWS Secrets Manager takes none.")
	attributes["id"] = resourcekit.IDAttribute(kind)
	attributes["version"] = resourcekit.VersionAttribute(kind)
	attributes["address"] = resourcekit.AddressAttribute("LINEAR_API_KEY")
	attributes["workspace_id"] = resourcekit.OwnerAttribute("workspace", kind)
	attributes["name"] = resourcekit.FixedAttribute("What integrations call the secret, as in `LINEAR_API_KEY`, written `secret.LINEAR_API_KEY` where one wears it.", kind)
	attributes["source"] = schema.SingleNestedAttribute{
		Optional:            true,
		Computed:            true,
		Default:             objectdefault.StaticValue(types.ObjectValueMust(sourceType, map[string]attr.Value{"kind": types.StringValue(spotlight.SourceBuiltIn), "secret_id": types.StringNull(), "field": types.StringNull()})),
		MarkdownDescription: "Where the value lives: kept by Spotlight (`built-in`, the default) or read from AWS Secrets Manager whenever it is used.",
		Attributes: map[string]schema.Attribute{
			"kind": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`built-in` or `aws-secrets-manager`.",
			},
			"secret_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The name or ARN of the secret in AWS Secrets Manager.",
			},
			"field": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The key to read when the AWS secret holds JSON.",
			},
		},
	}
	attributes["bindings"] = schema.ListNestedAttribute{
		Optional:            true,
		Computed:            true,
		Default:             listdefault.StaticValue(types.ListValueMust(bindingType, []attr.Value{})),
		MarkdownDescription: "The only hosts the secret is ever sent to, and how it is worn there; a request anywhere else goes without it.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"host": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: "The host, as in `api.linear.app`, with no scheme, port or path. The secret goes there over HTTPS only.",
				},
				"header": schema.StringAttribute{
					Optional:            true,
					Computed:            true,
					Default:             stringdefault.StaticString("authorization"),
					MarkdownDescription: "The request header the secret is worn in, in lower case.",
				},
				"prefix": schema.StringAttribute{
					Optional:            true,
					Computed:            true,
					Default:             stringdefault.StaticString(""),
					MarkdownDescription: "Written before the value in that header, as in `Bearer `.",
				},
			},
		},
	}
	attributes["rotated_at"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "When Spotlight last took a new value.",
	}

	return schema.Schema{
		MarkdownDescription: "A secret integrations wear toward the hosts it is bound to. Spotlight keeps its value and never answers it, so the value is write-only and never enters the plan or the state. " +
			"Writing secrets needs a token granted the `secrets` scope. Import it as `<workspace id>/<name>`.",
		Attributes: attributes,
	}
}

func inputOf(plan model) (spotlight.SecretInput, diag.Diagnostics) {
	in := spotlight.SecretInput{
		Name: plan.Name.ValueString(),
		Source: spotlight.SecretSource{
			Kind:     plan.Source.Kind.ValueString(),
			SecretID: plan.Source.SecretID.ValueString(),
			Field:    plan.Source.Field.ValueString(),
		},
		Bindings: make([]spotlight.SecretBinding, 0, len(plan.Bindings)),
	}

	for _, each := range plan.Bindings {
		in.Bindings = append(in.Bindings, spotlight.SecretBinding{
			Host:   each.Host.ValueString(),
			Header: each.Header.ValueString(),
			Prefix: each.Prefix.ValueString(),
		})
	}

	if !plan.ValueVersion.IsNull() && !plan.ValueVersion.IsUnknown() {
		version := plan.ValueVersion.ValueInt64()
		in.ValueVersion = &version
	}

	return in, nil
}

func sent(ctx context.Context, written resourcekit.Written, in *spotlight.SecretInput) diag.Diagnostics {
	var diags diag.Diagnostics

	if written.State != nil {
		rewritten, more := writeonly.Value.Rewritten(ctx, written.Plan, *written.State)
		diags.Append(more...)

		keptFromNow, more := becomesBuiltIn(ctx, *written.State, in.Source)
		diags.Append(more...)

		if !rewritten && !keptFromNow {
			return diags
		}
	}

	value, more := writeonly.Value.Read(ctx, written.Config)
	diags.Append(more...)

	if !value.IsNull() && !value.IsUnknown() {
		held := value.ValueString()
		in.Value = &held
	}

	return diags
}

func becomesBuiltIn(ctx context.Context, state tfsdk.State, planned spotlight.SecretSource) (bool, diag.Diagnostics) {
	var prior types.String
	diags := state.GetAttribute(ctx, path.Root("source").AtName("kind"), &prior)

	return planned.Kind == spotlight.SourceBuiltIn && prior.ValueString() != spotlight.SourceBuiltIn, diags
}

func settable(s spotlight.Secret) any {
	return map[string]any{
		"name":           s.Name,
		"source":         s.Source,
		"bindings":       s.Bindings,
		"valueWoVersion": s.ValueVersion,
	}
}

func stored(answer spotlight.Secret, kept *model) (model, diag.Diagnostics) {
	held := model{
		ID:          types.StringValue(answer.ID),
		Version:     types.Int64Value(answer.Version),
		Address:     types.StringValue(answer.Address),
		WorkspaceID: types.StringValue(answer.WorkspaceID),
		Name:        types.StringValue(answer.Name),
		Source: source{
			Kind:     types.StringValue(answer.Source.Kind),
			SecretID: resourcekit.TextOrNull(answer.Source.SecretID),
			Field:    resourcekit.TextOrNull(answer.Source.Field),
		},
		Bindings:     make([]binding, 0, len(answer.Bindings)),
		Value:        types.StringNull(),
		ValueVersion: types.Int64PointerValue(answer.ValueVersion),
		RotatedAt:    types.StringValue(answer.RotatedAt),
	}

	for _, each := range answer.Bindings {
		held.Bindings = append(held.Bindings, binding{
			Host:   types.StringValue(each.Host),
			Header: types.StringValue(each.Header),
			Prefix: types.StringValue(each.Prefix),
		})
	}

	if kept != nil {
		held.Name = kept.Name
		held.Source = kept.Source
		held.Bindings = kept.Bindings
		held.ValueVersion = kept.ValueVersion
	}

	return held, nil
}
