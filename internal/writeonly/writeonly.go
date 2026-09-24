// Package writeonly holds values the way Terraform 1.11 and later allow:
// written from configuration, sent to Spotlight, and never kept in the plan
// or the state. A version attribute beside each says when to send it again,
// since Terraform cannot see a change to a value it never keeps.
package writeonly

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Field is a write-only attribute and the version attribute beside it.
type Field struct {
	Name     string
	Version  string
	Required bool
}

// Value is the value a secret is written with.
var Value = Field{Name: "value_wo", Version: "value_wo_version"}

// Token is the token a runner calls with.
var Token = Field{Name: "token", Version: "token_version"}

// Attributes returns the field and its version, to add to the schema of a
// resource that holds it. described says what the value is.
func (f Field) Attributes(described string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		f.Name: schema.StringAttribute{
			Required:  f.Required,
			Optional:  !f.Required,
			Sensitive: true,
			WriteOnly: true,
			MarkdownDescription: described + " It is sent to Spotlight and never stored in the plan or the state, " +
				"so it needs Terraform 1.11 or later; change `" + f.Version + "` to send it again.",
		},
		f.Version: schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Any number; change it whenever `" + f.Name + "` changes, so Terraform sends the new value.",
		},
	}
}

// Read reads the value from configuration, the only place Terraform sends
// it; it is null when the configuration leaves it out.
func (f Field) Read(ctx context.Context, config tfsdk.Config) (types.String, diag.Diagnostics) {
	var value types.String
	diags := config.GetAttribute(ctx, path.Root(f.Name), &value)

	return value, diags
}

// Rewritten reports whether an update must send the value again, because
// its version changed.
func (f Field) Rewritten(ctx context.Context, plan tfsdk.Plan, state tfsdk.State) (bool, diag.Diagnostics) {
	var planned, prior types.Int64

	diags := plan.GetAttribute(ctx, path.Root(f.Version), &planned)
	diags.Append(state.GetAttribute(ctx, path.Root(f.Version), &prior)...)

	return !planned.Equal(prior), diags
}
