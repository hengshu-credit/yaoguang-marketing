package domain

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNullableJSON_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected NullableJSON
		wantErr  bool
	}{
		{
			name:  "nil value",
			input: nil,
			expected: NullableJSON{
				Data:   nil,
				IsNull: true,
			},
		},
		{
			name:  "empty byte slice",
			input: []byte{},
			expected: NullableJSON{
				Data:   nil,
				IsNull: true,
			},
		},
		{
			name:  "valid JSON object",
			input: []byte(`{"key":"value"}`),
			expected: NullableJSON{
				Data:   map[string]interface{}{"key": "value"},
				IsNull: false,
			},
		},
		{
			name:  "valid JSON array",
			input: []byte(`[1,2,3]`),
			expected: NullableJSON{
				Data:   []interface{}{float64(1), float64(2), float64(3)},
				IsNull: false,
			},
		},
		{
			name:    "invalid JSON",
			input:   []byte(`{invalid}`),
			wantErr: true,
		},
		{
			name:    "invalid type",
			input:   123,
			wantErr: true,
		},
		{
			name:  "empty string",
			input: "",
			expected: NullableJSON{
				Data:   nil,
				IsNull: true,
			},
		},
		{
			name:  "complex JSON object",
			input: []byte(`{"nested":{"key":"value"},"array":[1,2,3],"bool":true,"null":null}`),
			expected: NullableJSON{
				Data: map[string]interface{}{
					"nested": map[string]interface{}{"key": "value"},
					"array":  []interface{}{float64(1), float64(2), float64(3)},
					"bool":   true,
					"null":   nil,
				},
				IsNull: false,
			},
		},
		{
			name:  "string input",
			input: `{"key":"value"}`,
			expected: NullableJSON{
				Data:   map[string]interface{}{"key": "value"},
				IsNull: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nj NullableJSON
			err := nj.Scan(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, nj)
			}
		})
	}
}

func TestNullableJSON_Value(t *testing.T) {
	tests := []struct {
		name     string
		input    NullableJSON
		expected driver.Value
		wantErr  bool
	}{
		{
			name: "nil value",
			input: NullableJSON{
				Data:   nil,
				IsNull: true,
			},
			expected: nil,
		},
		{
			name: "valid object",
			input: NullableJSON{
				Data:   map[string]interface{}{"key": "value"},
				IsNull: false,
			},
			expected: []byte(`{"key":"value"}`),
		},
		{
			name: "valid array",
			input: NullableJSON{
				Data:   []interface{}{1, 2, 3},
				IsNull: false,
			},
			expected: []byte(`[1,2,3]`),
		},
		{
			name: "complex object",
			input: NullableJSON{
				Data: map[string]interface{}{
					"nested": map[string]interface{}{"key": "value"},
					"array":  []interface{}{1, 2, 3},
					"bool":   true,
					"null":   nil,
				},
				IsNull: false,
			},
			expected: []byte(`{"array":[1,2,3],"bool":true,"nested":{"key":"value"},"null":null}`),
		},
		{
			name: "empty object",
			input: NullableJSON{
				Data:   map[string]interface{}{},
				IsNull: false,
			},
			expected: []byte(`{}`),
		},
		{
			name: "empty array",
			input: NullableJSON{
				Data:   []interface{}{},
				IsNull: false,
			},
			expected: []byte(`[]`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := tt.input.Value()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expected == nil {
					assert.Nil(t, value)
				} else {
					assert.JSONEq(t, string(tt.expected.([]byte)), string(value.([]byte)))
				}
			}
		})
	}
}

func TestNullableJSON_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    NullableJSON
		expected string
		wantErr  bool
	}{
		{
			name: "null value",
			input: NullableJSON{
				Data:   nil,
				IsNull: true,
			},
			expected: "null",
		},
		{
			name: "valid object",
			input: NullableJSON{
				Data:   map[string]interface{}{"key": "value"},
				IsNull: false,
			},
			expected: `{"key":"value"}`,
		},
		{
			name: "valid array",
			input: NullableJSON{
				Data:   []interface{}{1, 2, 3},
				IsNull: false,
			},
			expected: `[1,2,3]`,
		},
		{
			name: "complex object",
			input: NullableJSON{
				Data: map[string]interface{}{
					"nested": map[string]interface{}{"key": "value"},
					"array":  []interface{}{1, 2, 3},
					"bool":   true,
					"null":   nil,
				},
				IsNull: false,
			},
			expected: `{"array":[1,2,3],"bool":true,"nested":{"key":"value"},"null":null}`,
		},
		{
			name: "empty object",
			input: NullableJSON{
				Data:   map[string]interface{}{},
				IsNull: false,
			},
			expected: `{}`,
		},
		{
			name: "empty array",
			input: NullableJSON{
				Data:   []interface{}{},
				IsNull: false,
			},
			expected: `[]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.JSONEq(t, tt.expected, string(data))
			}
		})
	}
}

func TestNullableJSON_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected NullableJSON
		wantErr  bool
	}{
		{
			name:  "null value",
			input: "null",
			expected: NullableJSON{
				Data:   nil,
				IsNull: true,
			},
		},
		{
			name:  "valid object",
			input: `{"key":"value"}`,
			expected: NullableJSON{
				Data:   map[string]interface{}{"key": "value"},
				IsNull: false,
			},
		},
		{
			name:  "valid array",
			input: `[1,2,3]`,
			expected: NullableJSON{
				Data:   []interface{}{float64(1), float64(2), float64(3)},
				IsNull: false,
			},
		},
		{
			name:  "complex object",
			input: `{"nested":{"key":"value"},"array":[1,2,3],"bool":true,"null":null}`,
			expected: NullableJSON{
				Data: map[string]interface{}{
					"nested": map[string]interface{}{"key": "value"},
					"array":  []interface{}{float64(1), float64(2), float64(3)},
					"bool":   true,
					"null":   nil,
				},
				IsNull: false,
			},
		},
		{
			name:  "empty object",
			input: `{}`,
			expected: NullableJSON{
				Data:   map[string]interface{}{},
				IsNull: false,
			},
		},
		{
			name:  "empty array",
			input: `[]`,
			expected: NullableJSON{
				Data:   []interface{}{},
				IsNull: false,
			},
		},
		{
			name:    "invalid JSON structure",
			input:   `{"key":}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nj NullableJSON
			err := json.Unmarshal([]byte(tt.input), &nj)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, nj)
			}
		})
	}
}

func TestNullableJSON_UnmarshalJSON_New(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected NullableJSON
		wantErr  bool
	}{
		{
			name:  "null value",
			input: []byte("null"),
			expected: NullableJSON{
				Data:   nil,
				IsNull: true,
			},
		},
		{
			name:  "empty object",
			input: []byte("{}"),
			expected: NullableJSON{
				Data:   map[string]interface{}{},
				IsNull: false,
			},
		},
		{
			name:  "object with values",
			input: []byte(`{"string":"value","number":42,"bool":true}`),
			expected: NullableJSON{
				Data: map[string]interface{}{
					"string": "value",
					"number": float64(42),
					"bool":   true,
				},
				IsNull: false,
			},
		},
		{
			name:  "array",
			input: []byte(`[1,"two",true]`),
			expected: NullableJSON{
				Data:   []interface{}{float64(1), "two", true},
				IsNull: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nj NullableJSON
			err := json.Unmarshal(tt.input, &nj)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, nj)
			}
		})
	}
}
