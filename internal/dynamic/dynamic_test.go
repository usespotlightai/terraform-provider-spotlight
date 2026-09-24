package dynamic_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/dynamic"
)

func document(t *testing.T, written string) any {
	t.Helper()

	var held any
	if err := json.Unmarshal([]byte(written), &held); err != nil {
		t.Fatal(err)
	}

	return held
}

func TestAJSONDocumentRoundTrips(t *testing.T) {
	written := document(t, `{"integration":"linear","port":8080,"ratio":0.5,"on":{"task.opened":{"do":"comment"}},"events":[{"when":"${{ true }}"},"x"],"empty":{},"none":[],"missing":null,"flag":true}`)

	value := dynamic.FromJSON(written)

	back, err := dynamic.ToJSON(value)
	if err != nil {
		t.Fatal(err)
	}

	one, _ := dynamic.Canonical(written)
	other, _ := dynamic.Canonical(back)
	if string(one) != string(other) {
		t.Errorf("round trip changed it:\n%s\n%s", one, other)
	}

	if _, err := value.ToTerraformValue(context.Background()); err != nil {
		t.Errorf("not a Terraform value: %v", err)
	}
}

func TestObjectsBecomeObjectsAndArraysTuples(t *testing.T) {
	value := dynamic.FromJSON(document(t, `{"list":["a",1]}`))

	object, ok := value.UnderlyingValue().(basetypes.ObjectValue)
	if !ok {
		t.Fatalf("got %T", value.UnderlyingValue())
	}

	if _, ok := object.Attributes()["list"].(basetypes.TupleValue); !ok {
		t.Errorf("got %T", object.Attributes()["list"])
	}
}

func TestCanonicalIgnoresOrderAndNumberSpelling(t *testing.T) {
	one, _ := dynamic.Canonical(map[string]any{"a": 1.0, "b": []any{"x"}})
	other, _ := dynamic.Canonical(document(t, `{"b":["x"],"a":1}`))

	if string(one) != string(other) {
		t.Errorf("%s != %s", one, other)
	}
}

func TestToJSONRefusesWhatIsNotKnown(t *testing.T) {
	value := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{"url": types.StringType},
		map[string]attr.Value{"url": types.StringUnknown()},
	))

	if _, err := dynamic.ToJSON(value); !errors.Is(err, dynamic.ErrUnknown) {
		t.Errorf("got %v", err)
	}

	held, complete := dynamic.ToJSONKnown(value)
	if complete || !reflect.DeepEqual(held, map[string]any{}) {
		t.Errorf("got %v, complete %v", held, complete)
	}
}

func TestResolveFillsOnlyWhatWasUnknown(t *testing.T) {
	planned := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{"url": types.StringType, "name": types.StringType, "items": types.TupleType{ElemTypes: []attr.Type{types.StringType}}},
		map[string]attr.Value{
			"url":   types.StringUnknown(),
			"name":  types.StringValue("as written"),
			"items": types.TupleValueMust([]attr.Type{types.StringType}, []attr.Value{types.StringUnknown()}),
		},
	))

	resolved := dynamic.Resolve(planned, document(t, `{"url":"https://x","name":"normalised","items":["filled"],"method":"POST"}`))

	back, err := dynamic.ToJSON(resolved)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{"url": "https://x", "name": "as written", "items": []any{"filled"}}
	if !reflect.DeepEqual(back, want) {
		t.Errorf("got %v, want %v", back, want)
	}

	if !resolved.UnderlyingValue().Type(context.Background()).Equal(planned.UnderlyingValue().Type(context.Background())) {
		t.Error("resolving changed the planned type")
	}
}

func TestResolveKeepsAKnownPlan(t *testing.T) {
	planned := dynamic.FromJSON(map[string]any{"integration": "linear"})

	if resolved := dynamic.Resolve(planned, map[string]any{"integration": "linear", "actions": map[string]any{}}); !resolved.Equal(planned) {
		t.Errorf("got %s", resolved)
	}
}
