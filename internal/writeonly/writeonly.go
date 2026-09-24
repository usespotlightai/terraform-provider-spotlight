// Package writeonly holds a secret's value the way Terraform 1.11 and later
// allow: written from configuration, sent to Spotlight, and never kept in
// the plan or the state. A version attribute beside it says when to write it
// again, since Terraform cannot see a value it never keeps change.
package writeonly

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The attributes a resource holding a secret value is written with.
const (
	ValueAttribute   = "value_wo"
	VersionAttribute = "value_wo_version"
)

// Attributes returns the value and version attributes, to add to the
// schema of a resource that holds a secret value.
func Attributes(what string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		ValueAttribute: schema.StringAttribute{
			Required:  true,
			Sensitive: true,
			WriteOnly: true,
			MarkdownDescription: "The " + what + ". It is sent to Spotlight and never stored in the plan or the state, " +
				"so it needs Terraform 1.11 or later; change `" + VersionAttribute + "` to send it again.",
		},
		VersionAttribute: schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Any number; change it whenever `" + ValueAttribute + "` changes, so Terraform sends the new value.",
		},
	}
}

// Value reads the value from configuration, the only place Terraform sends
// it.
func Value(ctx context.Context, config tfsdk.Config) (string, diag.Diagnostics) {
	var value types.String
	diags := config.GetAttribute(ctx, path.Root(ValueAttribute), &value)

	return value.ValueString(), diags
}

// Rewritten reports whether an update must send the value again, because
// its version changed.
func Rewritten(ctx context.Context, plan tfsdk.Plan, state tfsdk.State) (bool, diag.Diagnostics) {
	var planned, prior types.Int64

	diags := plan.GetAttribute(ctx, path.Root(VersionAttribute), &planned)
	diags.Append(state.GetAttribute(ctx, path.Root(VersionAttribute), &prior)...)

	return !planned.Equal(prior), diags
}
