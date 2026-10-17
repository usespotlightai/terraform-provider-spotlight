package dynamic

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Resolve returns the planned value. It fills each unknown part from the same
// place in the answer, and keeps each known part as planned.
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
		answered, _ := answer.(map[string]any)
		values := make(map[string]attr.Value, len(typed.Attributes()))
		for name, each := range typed.Attributes() {
			values[name] = resolve(each, answered[name])
		}
		return basetypes.NewObjectValueMust(typed.AttributeTypes(ctx), values)
	case basetypes.MapValue:
		answered, _ := answer.(map[string]any)
		values := make(map[string]attr.Value, len(typed.Elements()))
		for name, each := range typed.Elements() {
			values[name] = resolve(each, answered[name])
		}
		return basetypes.NewMapValueMust(typed.ElementType(ctx), values)
	case basetypes.TupleValue:
		answered, _ := answer.([]any)
		values := make([]attr.Value, len(typed.Elements()))
		for at, each := range typed.Elements() {
			values[at] = resolve(each, itemAt(answered, at))
		}
		return basetypes.NewTupleValueMust(typed.ElementTypes(ctx), values)
	case basetypes.ListValue:
		answered, _ := answer.([]any)
		values := make([]attr.Value, len(typed.Elements()))
		for at, each := range typed.Elements() {
			values[at] = resolve(each, itemAt(answered, at))
		}
		return basetypes.NewListValueMust(typed.ElementType(ctx), values)
	case basetypes.SetValue:
		if _, err := ToJSON(typed); err == nil {
			return typed
		}
		answered, _ := answer.([]any)
		values := make([]attr.Value, 0, len(answered))
		for _, each := range answered {
			values = append(values, typedAs(typed.ElementType(ctx), each))
		}
		return basetypes.NewSetValueMust(typed.ElementType(ctx), values)
	default:
		return planned
	}
}

func itemAt(items []any, at int) any {
	if at < len(items) {
		return items[at]
	}

	return nil
}

func typedAs(kind attr.Type, value any) attr.Value {
	switch kind.(type) {
	case basetypes.StringType:
		if value == nil {
			return types.StringNull()
		}
		if text, ok := value.(string); ok {
			return types.StringValue(text)
		}
		return types.StringValue(fmt.Sprint(value))
	case basetypes.BoolType:
		if flag, ok := value.(bool); ok {
			return types.BoolValue(flag)
		}
		return types.BoolNull()
	case basetypes.NumberType:
		if number, ok := valueOf(value).(basetypes.NumberValue); ok {
			return number
		}
		return types.NumberNull()
	default:
		return valueOf(value)
	}
}
