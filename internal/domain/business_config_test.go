package domain

import "testing"

func TestBusinessConfigArbitraryObject(t *testing.T) {
	d := BusinessDefinition{Kind: "object_type", Key: "contract", Name: "Contract", Status: "active", Schema: map[string]any{"type": "object", "properties": map[string]any{"number": map[string]any{"type": "string"}}}, Settings: map[string]any{}}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	d.Schema = map[string]any{"type": "imaginary"}
	if d.Validate() == nil {
		t.Fatal("invalid schema accepted")
	}
}
func TestBusinessConfigTypedValidation(t *testing.T) {
	schema := map[string]any{"type": "object", "required": []any{"amount"}, "properties": map[string]any{"amount": map[string]any{"type": "number"}}}
	if ValidateBusinessValue(schema, map[string]any{"amount": "wrong"}, "data") == nil {
		t.Fatal("string accepted as number")
	}
	if ValidateBusinessValue(schema, map[string]any{}, "data") == nil {
		t.Fatal("missing required field accepted")
	}
	if err := ValidateBusinessValue(schema, map[string]any{"amount": 1.2}, "data"); err != nil {
		t.Fatal(err)
	}
}
