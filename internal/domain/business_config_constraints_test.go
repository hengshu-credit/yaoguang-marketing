package domain

import "testing"

func TestBusinessConfigNullableAndConstraints(t *testing.T) {
	schema := map[string]any{"type": []any{"string", "null"}, "minLength": float64(3), "format": "email"}
	d := BusinessDefinition{Kind: "customer_attribute", Key: "email", Name: "Email", Schema: schema}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBusinessValue(schema, nil, "email"); err != nil {
		t.Fatal(err)
	}
	if ValidateBusinessValue(schema, "xx", "email") == nil {
		t.Fatal("constraints not enforced")
	}
	if ValidateBusinessValue(map[string]any{"type": "number", "minimum": float64(10)}, float64(1), "amount") == nil {
		t.Fatal("minimum ignored")
	}
}
