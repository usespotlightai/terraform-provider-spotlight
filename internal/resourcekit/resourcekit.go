// Package resourcekit holds what every Spotlight resource is built from:
// the clients the provider hands it, the attributes each carries, import
// ids, and the diagnostics a problem answer becomes.
package resourcekit

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/expressions"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/spotlight"
)

// Clients is what the provider hands each resource once it is configured.
type Clients struct {
	API         spotlight.API
	Expressions *expressions.Checker
}

// Configured reads the clients from a resource's configure request. It
// answers nil before the provider is configured, as during validation.
func Configured(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *Clients {
	if req.ProviderData == nil {
		return nil
	}

	clients, ok := req.ProviderData.(*Clients)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *resourcekit.Clients, got %T; this is a bug in the provider.", req.ProviderData))

		return nil
	}

	return clients
}

// Unconfigured adds the error for a resource used before the provider is.
func Unconfigured(diags *diag.Diagnostics) {
	diags.AddError("Provider not configured", "The Spotlight provider was not configured before this resource was used; this is a bug in the provider.")
}

// IDAttribute is the id Spotlight issues a resource when it is created.
func IDAttribute(kind string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: fmt.Sprintf("Issued by Spotlight when the %s is created, and never changed.", kind),
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

// VersionAttribute is the version that rises with each change.
func VersionAttribute(kind string) schema.Int64Attribute {
	return schema.Int64Attribute{
		Computed:            true,
		MarkdownDescription: fmt.Sprintf("Rises with every change to the %s; each write sends it back so a change made since is not overwritten.", kind),
	}
}

// AddressAttribute is the address that names a resource in its workspace.
func AddressAttribute(example string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: fmt.Sprintf("Names the resource within its workspace, as in `%s`.", example),
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

// SplitImportID reads an import id written as <workspaceId>/<address>.
func SplitImportID(id string) (workspaceID, address string, ok bool) {
	workspaceID, address, found := strings.Cut(strings.TrimSpace(id), "/")
	if !found || workspaceID == "" || address == "" {
		return "", "", false
	}

	return workspaceID, address, true
}

// BadImportID adds the error for an import id that is not written right.
func BadImportID(diags *diag.Diagnostics, id, example string) {
	diags.AddError("Unexpected import id", fmt.Sprintf("Import this resource by its workspace id and address, as in %q, not %q.", example, id))
}
