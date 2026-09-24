// Package dynamic converts between JSON documents and the values of a
// Terraform dynamic attribute, as an integration's definition is held.
package dynamic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// ErrUnknown says a value is not known yet, as during a plan.
var ErrUnknown = errors.New("the value is not known yet")

// FromJSON builds the value Terraform would hold for a JSON document written
// as an HCL literal: objects as objects, arrays as tuples.
func FromJSON(held any) types.Dynamic {
	return types.DynamicValue(valueOf(held))
}

func valueOf(held any) attr.Value {
	switch typed := held.(type) {
	case nil:
		return types.DynamicNull()
	case string:
		return types.StringValue(typed)
	case bool:
		return types.BoolValue(typed)
	case float64:
		return types.NumberValue(big.NewFloat(typed))
	case json.Number:
		number, _, err := big.ParseFloat(typed.String(), 10, 512, big.ToNearestEven)
		if err != nil {
			return types.StringValue(typed.String())
		}
		return types.NumberValue(number)
	case int:
		return types.NumberValue(new(big.Float).SetInt64(int64(typed)))
	case int64:
		return types.NumberValue(new(big.Float).SetInt64(typed))
	case []any:
		kinds := make([]attr.Type, len(typed))
		values := make([]attr.Value, len(typed))
		for at, each := range typed {
			values[at] = valueOf(each)
			kinds[at] = values[at].Type(context.Background())
		}
		return basetypes.NewTupleValueMust(kinds, values)
	case map[string]any:
		kinds := make(map[string]attr.Type, len(typed))
		values := make(map[string]attr.Value, len(typed))
		for name, each := range typed {
			values[name] = valueOf(each)
			kinds[name] = values[name].Type(context.Background())
		}
		return basetypes.NewObjectValueMust(kinds, values)
	case map[string]string:
		values := make(map[string]any, len(typed))
		for name, each := range typed {
			values[name] = each
		}
		return valueOf(values)
	default:
		return types.StringValue(fmt.Sprint(typed))
	}
}

// ToJSON writes a known value as a JSON document. It fails with ErrUnknown
// when any part of it is unknown.
func ToJSON(value attr.Value) (any, error) {
	return toJSON(value, func() (any, error) { return nil, ErrUnknown })
}

// ToJSONKnown writes a value as a JSON document, leaving out every part not
// known yet; complete says whether nothing was left out.
func ToJSONKnown(value attr.Value) (held any, complete bool) {
	complete = true
	held, _ = toJSON(value, func() (any, error) {
		complete = false
		return omitted{}, nil
	})

	return prune(held), complete
}

type omitted struct{}

func prune(held any) any {
	switch typed := held.(type) {
	case omitted:
		return nil
	case []any:
		for at, each := range typed {
			typed[at] = prune(each)
		}
		return typed
	case map[string]any:
		for name, each := range typed {
			if _, gone := each.(omitted); gone {
				delete(typed, name)
				continue
			}
			typed[name] = prune(each)
		}
		return typed
	default:
		return held
	}
}

func toJSON(value attr.Value, unknown func() (any, error)) (any, error) {
	if value == nil || value.IsNull() {
		return nil, nil
	}

	if value.IsUnknown() {
		return unknown()
	}

	switch typed := value.(type) {
	case basetypes.DynamicValue:
		return toJSON(typed.UnderlyingValue(), unknown)
	case basetypes.StringValue:
		return typed.ValueString(), nil
	case basetypes.BoolValue:
		return typed.ValueBool(), nil
	case basetypes.NumberValue:
		return numberOf(typed.ValueBigFloat()), nil
	case basetypes.Int64Value:
		return json.Number(fmt.Sprint(typed.ValueInt64())), nil
	case basetypes.Float64Value:
		return typed.ValueFloat64(), nil
	case basetypes.ObjectValue:
		return objectOf(typed.Attributes(), unknown)
	case basetypes.MapValue:
		return objectOf(typed.Elements(), unknown)
	case basetypes.ListValue:
		return arrayOf(typed.Elements(), unknown)
	case basetypes.SetValue:
		return arrayOf(typed.Elements(), unknown)
	case basetypes.TupleValue:
		return arrayOf(typed.Elements(), unknown)
	default:
		return nil, fmt.Errorf("a %s cannot be written as JSON", value.Type(context.Background()))
	}
}

func numberOf(number *big.Float) json.Number {
	if number.IsInt() {
		return json.Number(number.Text('f', 0))
	}

	return json.Number(number.Text('g', -1))
}

func objectOf(held map[string]attr.Value, unknown func() (any, error)) (any, error) {
	out := make(map[string]any, len(held))

	for name, each := range held {
		converted, err := toJSON(each, unknown)
		if err != nil {
			return nil, err
		}
		out[name] = converted
	}

	return out, nil
}

func arrayOf(held []attr.Value, unknown func() (any, error)) (any, error) {
	out := make([]any, 0, len(held))

	for _, each := range held {
		converted, err := toJSON(each, unknown)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}

	return out, nil
}

// Canonical writes a JSON document so two equal documents are written the
// same, whatever their key order or number representation.
func Canonical(held any) ([]byte, error) {
	raw, err := json.Marshal(held)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var normal any
	if err := decoder.Decode(&normal); err != nil {
		return nil, err
	}

	return json.Marshal(normalized(normal))
}

func normalized(held any) any {
	switch typed := held.(type) {
	case json.Number:
		number, _, err := big.ParseFloat(typed.String(), 10, 512, big.ToNearestEven)
		if err != nil {
			return typed
		}
		return numberOf(number)
	case []any:
		for at, each := range typed {
			typed[at] = normalized(each)
		}
		return typed
	case map[string]any:
		for name, each := range typed {
			typed[name] = normalized(each)
		}
		return typed
	default:
		return held
	}
}
