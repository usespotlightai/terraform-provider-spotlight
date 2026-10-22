package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
)

var (
	_ list.ListResource              = &OidcTrustListResource{}
	_ list.ListResourceWithConfigure = &OidcTrustListResource{}
)

// NewOidcTrustListResource returns the
// spotlight_oidc_trust list resource.
func NewOidcTrustListResource() list.ListResource {
	return &OidcTrustListResource{}
}

// OidcTrustListResource lists the OIDC trusts in a
// workspace.
type OidcTrustListResource struct {
	client *client.Client
}

// OidcTrustListResourceModel is the
// spotlight_oidc_trust list block's config.
type OidcTrustListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func (r *OidcTrustListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oidc_trust"
}

func (r *OidcTrustListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the OIDC trusts in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
		},
	}
}

func (r *OidcTrustListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *OidcTrustListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config OidcTrustListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	trusts, err := r.client.ListOidcTrusts(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the OIDC trusts", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, trust := range limited(trusts, req.Limit) {
			if !push(listResult(ctx, req, trust.ID, trust.Address, oidcTrustModel(trust, nil))) {
				return
			}
		}
	}
}
