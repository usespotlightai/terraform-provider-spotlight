package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type resourceIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

func idIdentitySchema(kind string) identityschema.Schema {
	return identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				RequiredForImport: true,
				Description:       fmt.Sprintf("The id Spotlight issued the %s when it was created.", kind),
			},
		},
	}
}

func setIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, id string) diag.Diagnostics {
	if identity == nil {
		return nil
	}

	return identity.Set(ctx, resourceIdentityModel{ID: types.StringValue(id)})
}

// removed removes from the state a resource that someone deleted outside
// Terraform. It still returns the identity of the resource. Terraform before
// 1.12 keeps no identity, and the framework rejects a read without identity.
func removed(ctx context.Context, resp *resource.ReadResponse, id string) {
	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, id)...)
	resp.State.RemoveResource(ctx)
}

// importTarget is the target of an import. For an import by identity, it is
// an id. For other imports, it is the workspace of the resource and its
// address in that workspace. The workspace is empty for a workspace, because
// the import of a workspace uses its slug.
type importTarget struct {
	id          string
	workspaceID string
	address     string
}

func (t importTarget) byID() bool { return t.id != "" }

func (t importTarget) String() string {
	switch {
	case t.id != "":
		return t.id
	case t.workspaceID == "":
		return t.address
	default:
		return t.workspaceID + "/" + t.address
	}
}

func importTargetOf(ctx context.Context, req resource.ImportStateRequest, diags *diag.Diagnostics, example string) (importTarget, bool) {
	if byIdentity(req) {
		return identityTarget(ctx, req, diags)
	}

	workspaceID, address, ok := splitImportID(req.ID)
	if !ok {
		diags.AddError("Unexpected import id", fmt.Sprintf("Import this resource by its workspace id and address, as in %q, not %q.", example, req.ID))

		return importTarget{}, false
	}

	return importTarget{workspaceID: workspaceID, address: address}, true
}

func slugImportTargetOf(ctx context.Context, req resource.ImportStateRequest, diags *diag.Diagnostics) (importTarget, bool) {
	if byIdentity(req) {
		return identityTarget(ctx, req, diags)
	}

	return importTarget{address: strings.TrimSpace(req.ID)}, true
}

func byIdentity(req resource.ImportStateRequest) bool {
	return req.ID == "" && req.Identity != nil
}

func identityTarget(ctx context.Context, req resource.ImportStateRequest, diags *diag.Diagnostics) (importTarget, bool) {
	var identity resourceIdentityModel
	diags.Append(req.Identity.Get(ctx, &identity)...)

	return importTarget{id: identity.ID.ValueString()}, !diags.HasError()
}

func splitImportID(id string) (workspaceID, address string, ok bool) {
	workspaceID, address, found := strings.Cut(strings.TrimSpace(id), "/")
	if !found || workspaceID == "" || address == "" {
		return "", "", false
	}

	return workspaceID, address, true
}
