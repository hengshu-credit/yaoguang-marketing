package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRootEmails(t *testing.T) {
	testCases := []struct {
		name     string
		setting  string
		expected []string
	}{
		{name: "empty", setting: "", expected: nil},
		{name: "whitespace only", setting: "   ", expected: nil},
		{name: "single email", setting: "alice@example.com", expected: []string{"alice@example.com"}},
		{
			name:     "comma separated",
			setting:  "alice@example.com,bob@example.com",
			expected: []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:     "semicolon separated",
			setting:  "alice@example.com;bob@example.com",
			expected: []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:     "mixed separators",
			setting:  "alice@example.com,bob@example.com;carol@example.com",
			expected: []string{"alice@example.com", "bob@example.com", "carol@example.com"},
		},
		{
			name:     "space separated",
			setting:  "alice@example.com bob@example.com",
			expected: []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:     "mixed comma and whitespace runs",
			setting:  "alice@example.com,  bob@example.com\tcarol@example.com",
			expected: []string{"alice@example.com", "bob@example.com", "carol@example.com"},
		},
		{
			name:     "surrounding spaces trimmed",
			setting:  " alice@example.com , bob@example.com ",
			expected: []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:     "trailing and empty entries dropped",
			setting:  "alice@example.com,,bob@example.com,",
			expected: []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:     "duplicates removed preserving first occurrence",
			setting:  "alice@example.com,bob@example.com,alice@example.com",
			expected: []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:     "case preserved (distinct entries)",
			setting:  "Alice@example.com,alice@example.com",
			expected: []string{"Alice@example.com", "alice@example.com"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, ParseRootEmails(tc.setting))
		})
	}
}

func TestIsRootEmail(t *testing.T) {
	testCases := []struct {
		name      string
		setting   string
		candidate string
		expected  bool
	}{
		{name: "single match (backward compat)", setting: "alice@example.com", candidate: "alice@example.com", expected: true},
		{name: "single no match", setting: "alice@example.com", candidate: "bob@example.com", expected: false},
		{name: "member of list", setting: "alice@example.com,bob@example.com", candidate: "bob@example.com", expected: true},
		{name: "non-member of list", setting: "alice@example.com,bob@example.com", candidate: "carol@example.com", expected: false},
		{name: "member with surrounding spaces in setting", setting: "alice@example.com, bob@example.com", candidate: "bob@example.com", expected: true},
		{name: "member in space-separated setting", setting: "alice@example.com bob@example.com", candidate: "bob@example.com", expected: true},
		{name: "case sensitive mismatch", setting: "Alice@example.com", candidate: "alice@example.com", expected: false},
		{name: "empty candidate never matches", setting: "alice@example.com", candidate: "", expected: false},
		{name: "empty setting never matches", setting: "", candidate: "alice@example.com", expected: false},
		{name: "empty candidate and empty setting", setting: "", candidate: "", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, IsRootEmail(tc.setting, tc.candidate))
		})
	}
}

func TestIsRootEmailInsensitive(t *testing.T) {
	testCases := []struct {
		name      string
		setting   string
		candidate string
		expected  bool
	}{
		{name: "exact match", setting: "alice@example.com", candidate: "alice@example.com", expected: true},
		{name: "case-folded match (config upper, candidate lower)", setting: "Alice@Example.com", candidate: "alice@example.com", expected: true},
		{name: "case-folded match (config lower, candidate upper)", setting: "alice@example.com", candidate: "ALICE@example.com", expected: true},
		{name: "member of list case-folded", setting: "Alice@example.com,Bob@example.com", candidate: "bob@example.com", expected: true},
		{name: "non-member", setting: "alice@example.com", candidate: "carol@example.com", expected: false},
		{name: "empty candidate never matches", setting: "alice@example.com", candidate: "", expected: false},
		{name: "empty setting never matches", setting: "", candidate: "alice@example.com", expected: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, IsRootEmailInsensitive(tc.setting, tc.candidate))
		})
	}
}

func TestPrimaryRootEmail(t *testing.T) {
	testCases := []struct {
		name     string
		setting  string
		expected string
	}{
		{name: "empty", setting: "", expected: ""},
		{name: "whitespace only", setting: " ; , ", expected: ""},
		{name: "single", setting: "alice@example.com", expected: "alice@example.com"},
		{name: "first of many", setting: "alice@example.com,bob@example.com", expected: "alice@example.com"},
		{name: "leading separator skipped", setting: ",bob@example.com", expected: "bob@example.com"},
		{name: "leading spaces trimmed", setting: "  alice@example.com , bob@example.com", expected: "alice@example.com"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, PrimaryRootEmail(tc.setting))
		})
	}
}

func TestConfigRootEmailMethods(t *testing.T) {
	c := &Config{RootEmail: "alice@example.com,bob@example.com"}

	assert.Equal(t, []string{"alice@example.com", "bob@example.com"}, c.RootEmails())
	assert.Equal(t, "alice@example.com", c.PrimaryRootEmail())
	assert.True(t, c.IsRootEmail("alice@example.com"))
	assert.True(t, c.IsRootEmail("bob@example.com"))
	assert.False(t, c.IsRootEmail("carol@example.com"))
	assert.False(t, c.IsRootEmail(""))

	empty := &Config{RootEmail: ""}
	assert.Nil(t, empty.RootEmails())
	assert.Equal(t, "", empty.PrimaryRootEmail())
	assert.False(t, empty.IsRootEmail("alice@example.com"))
}
