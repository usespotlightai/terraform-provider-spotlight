// Package runnertrust is the spotlight_runner_trust resource.
package runnertrust

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/resourcekit"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

const kind = "runner trust"

type model struct {
	ID          types.String `tfsdk:"id"`
	Version     types.Int64  `tfsdk:"version"`
	Address     types.String `tfsdk:"address"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
	Key         types.String `tfsdk:"key"`
	Issuer      types.String `tfsdk:"issuer"`
	Subject     types.String `tfsdk:"subject"`
	Audience    types.String `tfsdk:"audience"`
	Labels      types.Set    `tfsdk:"labels"`
}

// New returns the spotlight_runner_trust resource.
func New() resource.Resource {
	return resourcekit.NewManaged(resourcekit.Spec[spotlight.RunnerTrustInput, spotlight.RunnerTrust, model]{
		Kind:        "runner_trust",
		Schema:      schemaOf,
		AddressedAs: "production-cluster",
		Needs:       spotlight.ScopeRunners,
		Resources: func(api spotlight.API) spotlight.Resources[spotlight.RunnerTrustInput, spotlight.RunnerTrust] {
			return api.RunnerTrusts
		},
		Owner:    func(m model) string { return m.WorkspaceID.ValueString() },
		Input:    inputOf,
		Stored:   stored,
		Meta:     func(t spotlight.RunnerTrust) spotlight.Meta { return t.Meta },
		Settable: func(t spotlight.RunnerTrust) any { return t.RunnerTrustInput },
		Identity: func(m model) (string, int64) { return m.ID.ValueString(), m.Version.ValueInt64() },
	})
}

var schemaOf = schema.Schema{
	MarkdownDescription: "A workload whose platform-issued tokens enrol runners, as a Kubernetes service account's do: a runner there enrols without holding a Spotlight token. " +
		"Writing runner trusts needs a token granted the `runners` scope. Import it as `<workspace id>/<key>`.",
	Attributes: map[string]schema.Attribute{
		"id":           resourcekit.IDAttribute(kind),
		"version":      resourcekit.VersionAttribute(kind),
		"address":      resourcekit.AddressAttribute("production-cluster"),
		"workspace_id": resourcekit.OwnerAttribute("workspace", kind),
		"key":          resourcekit.FixedAttribute("Names the trust, as in `production-cluster`.", kind),
		"issuer": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The https URL the platform signs its workloads' tokens as; its keys are found through OIDC discovery.",
		},
		"subject": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The workload a token must be issued to, exactly, as in `system:serviceaccount:spotlight:runner`.",
		},
		"audience": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString("spotlight"),
			MarkdownDescription: "The audience a token must be issued for.",
		},
		"labels": resourcekit.StringSetAttribute("What a line or station may name in `runners` to place its work on the runners that enrol here, as in `gpu`."),
	},
}

func inputOf(plan model) (spotlight.RunnerTrustInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	return spotlight.RunnerTrustInput{
		Key:      plan.Key.ValueString(),
		Issuer:   plan.Issuer.ValueString(),
		Subject:  plan.Subject.ValueString(),
		Audience: plan.Audience.ValueString(),
		Labels:   resourcekit.StringsIn(plan.Labels, &diags),
	}, diags
}

func stored(answer spotlight.RunnerTrust, kept *model) (model, diag.Diagnostics) {
	held := model{
		ID:          types.StringValue(answer.ID),
		Version:     types.Int64Value(answer.Version),
		Address:     types.StringValue(answer.Address),
		WorkspaceID: types.StringValue(answer.WorkspaceID),
		Key:         types.StringValue(answer.Key),
		Issuer:      types.StringValue(answer.Issuer),
		Subject:     types.StringValue(answer.Subject),
		Audience:    types.StringValue(answer.Audience),
		Labels:      resourcekit.StringSet(answer.Labels),
	}

	if kept != nil {
		held.Key = kept.Key
		held.Issuer = kept.Issuer
		held.Subject = kept.Subject
		held.Audience = kept.Audience
		held.Labels = kept.Labels
	}

	return held, nil
}
