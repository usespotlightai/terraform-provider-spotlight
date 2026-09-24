package dynamic

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Resolve returns what was planned with each part that was unknown filled in
// from the same place in the answer, keeping every known part as planned.
func Resolve(planned types.Dynamic, answer any) types.Dynamic {
	if planned.IsUnknown() || planned.IsUnderlyingValueUnknown() {
		return FromJSON(answer)
	}

	if planned.IsNull() {
		return planned
	}

	return types.DynamicValue(resolve(planned.UnderlyingValue(), answer))
}

func resolve(planned attr.Value, answer any) attr.Value {
	if planned.IsUnknown() {
		return typedAs(planned.Type(context.Background()), answer)
	}

	if planned.IsNull() {
		return planned
	}

	ctx := context.Background()

	switch typed := planned.(type) {
	case basetypes.DynamicValue:
		return types.DynamicValue(resolve(typed.UnderlyingValue(), answer))
	case basetypes.ObjectValue:
		held, _ := answer.(map[string]any)
		values := make(map[string]attr.Value, len(typed.Attributes()))
		for name, each := range typed.Attributes() {
			values[name] = resolve(each, held[name])
		}
		return basetypes.NewObjectValueMust(typed.AttributeTypes(ctx), values)
	case basetypes.MapValue:
		held, _ := answer.(map[string]any)
		values := make(map[string]attr.Value, len(typed.Elements()))
		for name, each := range typed.Elements() {
			values[name] = resolve(each, held[name])
		}
		return basetypes.NewMapValueMust(typed.ElementType(ctx), values)
	case basetypes.TupleValue:
		held, _ := answer.([]any)
		values := make([]attr.Value, len(typed.Elements()))
		for at, each := range typed.Elements() {
			values[at] = resolve(each, itemAt(held, at))
		}
		return basetypes.NewTupleValueMust(typed.ElementTypes(ctx), values)
	case basetypes.ListValue:
		held, _ := answer.([]any)
		values := make([]attr.Value, len(typed.Elements()))
		for at, each := range typed.Elements() {
			values[at] = resolve(each, itemAt(held, at))
		}
		return basetypes.NewListValueMust(typed.ElementType(ctx), values)
	case basetypes.SetValue:
		if _, err := ToJSON(typed); err == nil {
			return typed
		}
		held, _ := answer.([]any)
		values := make([]attr.Value, 0, len(held))
		for _, each := range held {
			values = append(values, typedAs(typed.ElementType(ctx), each))
		}
		return basetypes.NewSetValueMust(typed.ElementType(ctx), values)
	default:
		return planned
	}
}

func itemAt(held []any, at int) any {
	if at < len(held) {
		return held[at]
	}

	return nil
}

func typedAs(kind attr.Type, held any) attr.Value {
	switch kind.(type) {
	case basetypes.StringType:
		if held == nil {
			return types.StringNull()
		}
		if text, ok := held.(string); ok {
			return types.StringValue(text)
		}
		return types.StringValue(fmt.Sprint(held))
	case basetypes.BoolType:
		if flag, ok := held.(bool); ok {
			return types.BoolValue(flag)
		}
		return types.BoolNull()
	case basetypes.NumberType:
		if number, ok := valueOf(held).(basetypes.NumberValue); ok {
			return number
		}
		return types.NumberNull()
	default:
		return valueOf(held)
	}
}
