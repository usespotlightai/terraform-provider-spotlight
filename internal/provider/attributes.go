package provider

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

func idAttribute(kind string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: fmt.Sprintf("Issued by Spotlight when the %s is created, and never changed.", kind),
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func versionAttribute(kind string) schema.Int64Attribute {
	return schema.Int64Attribute{
		Computed:            true,
		MarkdownDescription: fmt.Sprintf("Rises with every change to the %s; each write sends it back so a change made since is not overwritten.", kind),
	}
}

func addressAttribute(example string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: fmt.Sprintf("Names the resource within its workspace, as in `%s`.", example),
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func ownerAttribute(owner, kind string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: fmt.Sprintf("The %s the %s belongs to. Changing it replaces the %s.", owner, kind, kind),
		PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
}

func fixedAttribute(description, kind string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: fmt.Sprintf("%s It is fixed for the %s's life, so changing it replaces the %s.", description, kind, kind),
		PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
}

func stringSetAttribute(description string) schema.SetAttribute {
	return schema.SetAttribute{
		Optional:            true,
		Computed:            true,
		ElementType:         types.StringType,
		Default:             setdefault.StaticValue(stringSet(nil)),
		MarkdownDescription: description,
	}
}

func stringListAttribute(description string) schema.ListAttribute {
	return schema.ListAttribute{
		Optional:            true,
		Computed:            true,
		ElementType:         types.StringType,
		Default:             listdefault.StaticValue(stringList(nil)),
		MarkdownDescription: description,
	}
}

type collection interface {
	IsNull() bool
	IsUnknown() bool
	ElementsAs(ctx context.Context, target any, allowUnhandled bool) diag.Diagnostics
}

func stringsIn(held collection, diags *diag.Diagnostics) []string {
	out := []string{}
	if held.IsNull() || held.IsUnknown() {
		return out
	}

	diags.Append(held.ElementsAs(context.Background(), &out, false)...)

	return out
}

func textOrNull(held string) types.String {
	if held == "" {
		return types.StringNull()
	}

	return types.StringValue(held)
}

func stringSet(held []string) types.Set {
	return types.SetValueMust(types.StringType, stringValues(held))
}

func stringList(held []string) types.List {
	return types.ListValueMust(types.StringType, stringValues(held))
}

func stringMap(held map[string]string) types.Map {
	out := make(map[string]attr.Value, len(held))
	for key, each := range held {
		out[key] = types.StringValue(each)
	}

	return types.MapValueMust(types.StringType, out)
}

func stringValues(held []string) []attr.Value {
	out := make([]attr.Value, 0, len(held))
	for _, each := range held {
		out = append(out, types.StringValue(each))
	}

	return out
}
