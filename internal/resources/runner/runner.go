// Package runner is the spotlight_runner resource.
package runner

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/writeonly"
)

const kind = "runner"

// IssuedToken is the warning a runner created without a token is answered
// with: Spotlight issued one, and Terraform kept none of it.
const IssuedToken = "Spotlight issued this runner a token, answered only once. Terraform keeps no secret in its state, so the token was not kept, and no runner can call with it. " +
	"Give the runner a token you hold instead: set `token` (from a sensitive variable, or a write-only ephemeral value) and change `token_version` to send it. " +
	"A runner on a platform that issues its workloads tokens can enrol through a `spotlight_runner_trust` rather than hold a token."

type model struct {
	ID           types.String `tfsdk:"id"`
	Version      types.Int64  `tfsdk:"version"`
	Address      types.String `tfsdk:"address"`
	WorkspaceID  types.String `tfsdk:"workspace_id"`
	Name         types.String `tfsdk:"name"`
	Labels       types.Set    `tfsdk:"labels"`
	Token        types.String `tfsdk:"token"`
	TokenVersion types.Int64  `tfsdk:"token_version"`
	Trust        types.String `tfsdk:"trust"`
}

// New returns the spotlight_runner resource.
func New() resource.Resource {
	return resourcekit.NewManaged(resourcekit.Spec[spotlight.RunnerInput, spotlight.Runner, model]{
		Kind:        kind,
		Schema:      schemaOf(),
		AddressedAs: "build-1",
		Needs:       spotlight.ScopeRunners,
		Resources: func(api spotlight.API) spotlight.Resources[spotlight.RunnerInput, spotlight.Runner] {
			return api.Runners
		},
		Owner:    func(m model) string { return m.WorkspaceID.ValueString() },
		Input:    inputOf,
		Sent:     sent,
		Created:  created,
		Stored:   stored,
		Meta:     func(r spotlight.Runner) spotlight.Meta { return r.Meta },
		Settable: func(r spotlight.Runner) any { return spotlight.RunnerInput{Name: r.Name, Labels: r.Labels} },
		Identity: func(m model) (string, int64) { return m.ID.ValueString(), m.Version.ValueInt64() },
	})
}

func schemaOf() schema.Schema {
	attributes := writeonly.Token.Attributes("The token the runner calls with: 32 to 255 URL-safe characters, as a generated password would be. " +
		"Spotlight keeps only its hash. Left out, Spotlight issues one when it creates the runner, which Terraform does not keep.")
	attributes["id"] = resourcekit.IDAttribute(kind)
	attributes["version"] = resourcekit.VersionAttribute(kind)
	attributes["address"] = resourcekit.AddressAttribute("build-1")
	attributes["workspace_id"] = resourcekit.OwnerAttribute("workspace", kind)
	attributes["name"] = resourcekit.FixedAttribute("What the runner is called, and what a line or station names in `runners` to place its work on it.", kind)
	attributes["labels"] = resourcekit.StringSetAttribute("What a line or station may name in `runners` to place its work here, beside the runner's name, as in `vpc-internal`.")
	attributes["trust"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The key of the runner trust the runner enrolled through, or null when it holds a token.",
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}

	return schema.Schema{
		MarkdownDescription: "A runner that calls Spotlight with a token, to claim and run its workspace's work. " +
			"Writing runners needs a token granted the `runners` scope. Import it as `<workspace id>/<name>`.",
		Attributes: attributes,
	}
}

func inputOf(plan model) (spotlight.RunnerInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	return spotlight.RunnerInput{
		Name:   plan.Name.ValueString(),
		Labels: resourcekit.StringsIn(plan.Labels, &diags),
	}, diags
}

func sent(ctx context.Context, written resourcekit.Written, in *spotlight.RunnerInput) diag.Diagnostics {
	var diags diag.Diagnostics

	if written.State != nil {
		rewritten, more := writeonly.Token.Rewritten(ctx, written.Plan, *written.State)
		diags.Append(more...)

		if !rewritten {
			return diags
		}
	}

	token, more := writeonly.Token.Read(ctx, written.Config)
	diags.Append(more...)

	if !token.IsNull() && !token.IsUnknown() {
		in.Token = token.ValueString()
	}

	return diags
}

func created(answer spotlight.Runner) diag.Diagnostics {
	var diags diag.Diagnostics

	if answer.IssuedToken {
		diags.AddAttributeWarning(path.Root(writeonly.Token.Name), "Spotlight issued the runner a token Terraform did not keep", IssuedToken)
	}

	return diags
}

func stored(answer spotlight.Runner, kept *model) (model, diag.Diagnostics) {
	held := model{
		ID:           types.StringValue(answer.ID),
		Version:      types.Int64Value(answer.Version),
		Address:      types.StringValue(answer.Address),
		WorkspaceID:  types.StringValue(answer.WorkspaceID),
		Name:         types.StringValue(answer.Name),
		Labels:       resourcekit.StringSet(answer.Labels),
		Token:        types.StringNull(),
		TokenVersion: types.Int64Null(),
		Trust:        resourcekit.TextOrNull(answer.Trust),
	}

	if kept != nil {
		held.Name = kept.Name
		held.Labels = kept.Labels
		held.TokenVersion = kept.TokenVersion
	}

	return held, nil
}
