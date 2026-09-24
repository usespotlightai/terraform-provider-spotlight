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
	_ list.ListResource              = &SecretListResource{}
	_ list.ListResourceWithConfigure = &SecretListResource{}
)

// NewSecretListResource returns the spotlight_secret list resource.
func NewSecretListResource() list.ListResource {
	return &SecretListResource{}
}

// SecretListResource lists the secrets in a workspace.
type SecretListResource struct {
	client *client.Client
}

// SecretListResourceModel is the spotlight_secret list block's config.
type SecretListResourceModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func (r *SecretListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (r *SecretListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the secrets in a workspace.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": workspaceIDListAttribute(),
		},
	}
}

func (r *SecretListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientOf(req.ProviderData, &resp.Diagnostics)
}

func (r *SecretListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config SecretListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	secrets, err := r.client.ListSecrets(ctx, config.WorkspaceID.ValueString())
	if err != nil {
		stream.Results = listFailed("Could not list the secrets", err)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, secret := range limited(secrets, req.Limit) {
			if !push(listResult(ctx, req, secret.ID, secret.Address, secretModel(secret, nil))) {
				return
			}
		}
	}
}
