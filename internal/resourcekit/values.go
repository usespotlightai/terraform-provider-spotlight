package resourcekit

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Collection is a Terraform list or set.
type Collection interface {
	IsNull() bool
	IsUnknown() bool
	ElementsAs(ctx context.Context, target any, allowUnhandled bool) diag.Diagnostics
}

// StringsIn reads the strings a list or set holds, none when it is null or
// not yet known.
func StringsIn(held Collection, diags *diag.Diagnostics) []string {
	out := []string{}
	if held.IsNull() || held.IsUnknown() {
		return out
	}

	diags.Append(held.ElementsAs(context.Background(), &out, false)...)

	return out
}

// TextOrNull holds text Spotlight answers empty for none as a Terraform
// string, null when it is empty.
func TextOrNull(held string) types.String {
	if held == "" {
		return types.StringNull()
	}

	return types.StringValue(held)
}

// StringSet holds strings as a Terraform set.
func StringSet(held []string) types.Set {
	return types.SetValueMust(types.StringType, stringValues(held))
}

// StringList holds strings as a Terraform list.
func StringList(held []string) types.List {
	return types.ListValueMust(types.StringType, stringValues(held))
}

func stringValues(held []string) []attr.Value {
	out := make([]attr.Value, 0, len(held))
	for _, each := range held {
		out = append(out, types.StringValue(each))
	}

	return out
}

// OwnerAttribute is the id of the workspace or team a resource belongs to,
// which it keeps for its life.
func OwnerAttribute(owner, kind string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: fmt.Sprintf("The %s the %s belongs to. Changing it replaces the %s.", owner, kind, kind),
		PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
}

// FixedAttribute is a name a resource keeps for its life, so that changing
// it replaces the resource.
func FixedAttribute(description, kind string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: fmt.Sprintf("%s It is fixed for the %s's life, so changing it replaces the %s.", description, kind, kind),
		PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
}

// StringSetAttribute is a set of strings that is empty when left out.
func StringSetAttribute(description string) schema.SetAttribute {
	return schema.SetAttribute{
		Optional:            true,
		Computed:            true,
		ElementType:         types.StringType,
		Default:             setdefault.StaticValue(StringSet(nil)),
		MarkdownDescription: description,
	}
}

// StringListAttribute is a list of strings that is empty when left out.
func StringListAttribute(description string) schema.ListAttribute {
	return schema.ListAttribute{
		Optional:            true,
		Computed:            true,
		ElementType:         types.StringType,
		Default:             listdefault.StaticValue(StringList(nil)),
		MarkdownDescription: description,
	}
}
