// Package writeonly holds values as Terraform 1.11 and later versions permit.
// The provider reads each value from configuration and sends it to Spotlight.
// The plan and the state never keep it. A version attribute next to each
// value tells when to send it again. Terraform cannot see a change to a value
// that it does not keep.
package writeonly

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Field is a write-only attribute and the version attribute next to it, at
// the root or Within a nested attribute. If FromOne is true, the version is a
// whole number from 1.
type Field struct {
	Within   string
	Name     string
	Version  string
	Required bool
	FromOne  bool
}

// Value is the value that the provider writes to a secret.
var Value = Field{Name: "value_wo", Version: "value_wo_version"}

// Token is the token that the hosts of a pull pool use to claim work.
var Token = Field{Name: "token", Version: "token_version"}

// EgressToken is the token that an egress calls Spotlight with.
var EgressToken = Field{Name: "token", Version: "token_version"}

// ProvisionSecret is the secret that a push pool signs its provision calls
// with.
var ProvisionSecret = Field{Within: "provision", Name: "secret", Version: "secret_version", Required: true}

// SigningSecret is the HMAC secret that a hook signs its requests with.
var SigningSecret = Field{Within: "signing", Name: "secret_wo", Version: "secret_wo_version", FromOne: true}

// Path returns the Terraform path of the write-only attribute.
func (f Field) Path() path.Path {
	return f.at(f.Name)
}

func (f Field) at(name string) path.Path {
	if f.Within == "" {
		return path.Root(name)
	}

	return path.Root(f.Within).AtName(name)
}

// Attributes returns the field and its version for the schema of a resource
// that holds the field. described tells what the value is.
func (f Field) Attributes(described string) map[string]schema.Attribute {
	version := schema.Int64Attribute{
		Optional:            true,
		MarkdownDescription: "Any number; change it whenever `" + f.Name + "` changes, so Terraform sends the new value.",
	}
	if f.FromOne {
		version.Validators = []validator.Int64{int64validator.AtLeast(1)}
		version.MarkdownDescription = "A whole number from 1; change it whenever `" + f.Name + "` changes, so Terraform sends the new value."
	}

	return map[string]schema.Attribute{
		f.Name: schema.StringAttribute{
			Required:  f.Required,
			Optional:  !f.Required,
			Sensitive: true,
			WriteOnly: true,
			MarkdownDescription: described + " It is sent to Spotlight and never stored in the plan or the state, " +
				"so it needs Terraform 1.11 or later; change `" + f.Version + "` to send it again.",
		},
		f.Version: version,
	}
}

// Read reads the value from configuration, the only place where Terraform
// sends it. The value is null if the configuration does not contain it.
func (f Field) Read(ctx context.Context, config tfsdk.Config) (types.String, diag.Diagnostics) {
	var value types.String
	diags := config.GetAttribute(ctx, f.Path(), &value)

	return value, diags
}

// Rewritten reports if an update must send the value again because its
// version changed.
func (f Field) Rewritten(ctx context.Context, plan tfsdk.Plan, state tfsdk.State) (bool, diag.Diagnostics) {
	var planned, prior types.Int64

	diags := plan.GetAttribute(ctx, f.at(f.Version), &planned)
	diags.Append(state.GetAttribute(ctx, f.at(f.Version), &prior)...)

	return !planned.Equal(prior), diags
}
