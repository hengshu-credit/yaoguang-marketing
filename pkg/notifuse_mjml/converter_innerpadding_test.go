package notifuse_mjml

import (
	"strings"
	"testing"
)

// TestFormatSingleAttribute_BoxModelMap guards against issue #369: the visual
// editor can store padding-like attributes (e.g. mj-button inner-padding) as an
// object {top, right, bottom, left}. When such a value reached the converter it
// was formatted with fmt.Sprintf("%v", ...), leaking Go's "map[bottom:0px top:0px]"
// representation into the MJML/CSS and causing strict clients (Gmail) to drop the
// element. A box-model object must compile to a valid CSS shorthand instead.
func TestFormatSingleAttribute_BoxModelMap(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    interface{}
		expected string
	}{
		{
			name:     "partial box model (top+bottom) collapses to a single value",
			key:      "innerPadding",
			value:    map[string]interface{}{"top": "0px", "bottom": "0px"},
			expected: ` inner-padding="0px"`,
		},
		{
			name:     "vertical/horizontal collapses to two values",
			key:      "innerPadding",
			value:    map[string]interface{}{"top": "10px", "right": "25px", "bottom": "10px", "left": "25px"},
			expected: ` inner-padding="10px 25px"`,
		},
		{
			name:     "four distinct sides use the full shorthand",
			key:      "padding",
			value:    map[string]interface{}{"top": "1px", "right": "2px", "bottom": "3px", "left": "4px"},
			expected: ` padding="1px 2px 3px 4px"`,
		},
		{
			name:     "single side defaults the remaining sides to 0px",
			key:      "innerPadding",
			value:    map[string]interface{}{"top": "5px"},
			expected: ` inner-padding="5px 0px 0px 0px"`,
		},
		{
			name:     "non box-model map is skipped, never emitted as a Go map literal",
			key:      "data",
			value:    map[string]interface{}{"foo": "bar"},
			expected: ``,
		},
		{
			name:     "empty map is skipped",
			key:      "innerPadding",
			value:    map[string]interface{}{},
			expected: ``,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSingleAttribute(tt.key, tt.value)
			if got != tt.expected {
				t.Errorf("formatSingleAttribute(%q, %v) = %q, want %q", tt.key, tt.value, got, tt.expected)
			}
			if strings.Contains(got, "map[") {
				t.Errorf("formatSingleAttribute(%q, %v) leaked a Go map literal: %q", tt.key, tt.value, got)
			}
		})
	}
}

// TestConvertButton_InnerPaddingObject_NoGoMapLiteral reproduces the exact issue
// #369 payload end-to-end: an mj-button whose inner padding was stored as an
// object must compile to valid MJML with no "map[...]" leakage.
func TestConvertButton_InnerPaddingObject_NoGoMapLiteral(t *testing.T) {
	buttonBase := NewBaseBlock("button-1", MJMLComponentMjButton)
	buttonBase.Attributes["innerPadding"] = map[string]interface{}{"top": "0px", "bottom": "0px"}
	buttonBase.Content = stringPtr("Fórum E-commerce 2026")
	buttonBlock := &MJButtonBlock{BaseBlock: buttonBase}

	mjml := ConvertJSONToMJML(buttonBlock)

	if strings.Contains(mjml, "map[") {
		t.Fatalf("MJML output leaked a Go map literal: %s", mjml)
	}
	if !strings.Contains(mjml, `inner-padding="0px"`) {
		t.Errorf("expected inner-padding=\"0px\" in output, got: %s", mjml)
	}
}
