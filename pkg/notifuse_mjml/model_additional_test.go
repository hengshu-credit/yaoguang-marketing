package notifuse_mjml

import "testing"

func TestMarshalEmailBlock(t *testing.T) {
	// nil should marshal to null
	b, err := MarshalEmailBlock(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(b) != "null" {
		t.Fatalf("expected 'null', got %s", string(b))
	}
}

func TestUnmarshalEmailBlocks(t *testing.T) {
	data := `[
        {"id":"t1","type":"mj-text","content":"hi"},
        {"id":"btn1","type":"mj-button","content":"go"}
    ]`
	blocks, err := UnmarshalEmailBlocks([]byte(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].GetID() != "t1" || blocks[1].GetID() != "btn1" {
		t.Fatalf("unexpected IDs: %s, %s", blocks[0].GetID(), blocks[1].GetID())
	}
}

func TestConvertMapToTypedAttributes_CreateBlockWithDefaults_ValidateAndFixAttributes(t *testing.T) {
	// ConvertMapToTypedAttributes with nil
	if err := ConvertMapToTypedAttributes(nil, &MJTextAttributes{}); err != nil {
		t.Fatalf("unexpected error for nil attrs: %v", err)
	}

	// CreateBlockWithDefaults merges
	merged := CreateBlockWithDefaults(MJMLComponentMjButton, map[string]interface{}{"color": "#111"})
	if merged["color"] != "#111" { // override default color
		t.Fatalf("expected override color, got %v", merged["color"])
	}
	if merged["backgroundColor"] == nil { // default exists
		t.Fatalf("expected default backgroundColor")
	}

	// ValidateAndFixAttributes adds missing defaults but keeps provided
	fixed := ValidateAndFixAttributes(MJMLComponentMjText, map[string]interface{}{"fontSize": "18px"})
	if fixed["fontSize"] != "18px" {
		t.Fatalf("expected provided fontSize preserved")
	}
	if fixed["color"] == nil { // default added
		t.Fatalf("expected default color added")
	}
}
