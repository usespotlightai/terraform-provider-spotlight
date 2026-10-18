package provider

import (
	"context"
	"fmt"
	"math/big"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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

func stringsIn(values collection, diags *diag.Diagnostics) []string {
	out := []string{}
	if values.IsNull() || values.IsUnknown() {
		return out
	}

	diags.Append(values.ElementsAs(context.Background(), &out, false)...)

	return out
}

func textOrNull(text string) types.String {
	if text == "" {
		return types.StringNull()
	}

	return types.StringValue(text)
}

func float64AsWritten(number float64) types.Float64 {
	written, _, err := big.ParseFloat(strconv.FormatFloat(number, 'g', -1, 64), 10, 512, big.ToNearestEven)
	if err != nil {
		return types.Float64Value(number)
	}

	value, err := types.Float64Type.ValueFromTerraform(context.Background(), tftypes.NewValue(tftypes.Number, written))
	read, ok := value.(types.Float64)
	if err != nil || !ok {
		return types.Float64Value(number)
	}

	return read
}

type float64Default float64

func (d float64Default) Description(_ context.Context) string {
	return fmt.Sprintf("value defaults to %v", float64(d))
}

func (d float64Default) MarkdownDescription(ctx context.Context) string {
	return d.Description(ctx)
}

func (d float64Default) DefaultFloat64(_ context.Context, _ defaults.Float64Request, resp *defaults.Float64Response) {
	resp.PlanValue = float64AsWritten(float64(d))
}

func stringSet(values []string) types.Set {
	return types.SetValueMust(types.StringType, stringValues(values))
}

func stringList(values []string) types.List {
	return types.ListValueMust(types.StringType, stringValues(values))
}

func stringMap(values map[string]string) types.Map {
	out := make(map[string]attr.Value, len(values))
	for key, each := range values {
		out[key] = types.StringValue(each)
	}

	return types.MapValueMust(types.StringType, out)
}

func stringValues(values []string) []attr.Value {
	out := make([]attr.Value, 0, len(values))
	for _, each := range values {
		out = append(out, types.StringValue(each))
	}

	return out
}
