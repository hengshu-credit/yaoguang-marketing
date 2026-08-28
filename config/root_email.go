package config

import (
	"slices"
	"strings"
	"unicode"
)

// ParseRootEmails splits a ROOT_EMAIL setting into a de-duplicated,
// order-preserving list of non-empty emails. Entries may be separated by commas,
// semicolons, or whitespace (email addresses never contain unquoted whitespace,
// and the console tag input also treats spaces as separators). Case is preserved
// (matching is case-sensitive).
func ParseRootEmails(setting string) []string {
	if setting == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, e := range strings.FieldsFunc(setting, func(r rune) bool {
		return r == ',' || r == ';' || unicode.IsSpace(r)
	}) {
		if _, dup := seen[e]; dup {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	return out
}

// IsRootEmail reports whether candidate exactly matches one of the configured root
// emails (case-sensitive). An empty candidate never matches.
func IsRootEmail(setting, candidate string) bool {
	if candidate == "" {
		return false
	}
	return slices.Contains(ParseRootEmails(setting), candidate)
}

// PrimaryRootEmail returns the first configured root email, or "" if none.
func PrimaryRootEmail(setting string) string {
	emails := ParseRootEmails(setting)
	if len(emails) == 0 {
		return ""
	}
	return emails[0]
}

// IsRootEmail reports whether email is one of the configured root emails.
func (c *Config) IsRootEmail(email string) bool { return IsRootEmail(c.RootEmail, email) }

// IsRootEmailInsensitive reports whether candidate matches a configured root email,
// comparing case-insensitively. The OIDC path uses this (not the case-sensitive
// IsRootEmail) because the IdP-asserted email's casing may differ from the
// configured ROOT_EMAIL — a case-sensitive miss there would silently disable the
// root-account privilege-escalation guard.
func IsRootEmailInsensitive(setting, candidate string) bool {
	if candidate == "" {
		return false
	}
	for _, r := range ParseRootEmails(setting) {
		if strings.EqualFold(r, candidate) {
			return true
		}
	}
	return false
}

// IsRootEmailInsensitive reports whether email matches a configured root email,
// case-insensitively.
func (c *Config) IsRootEmailInsensitive(email string) bool {
	return IsRootEmailInsensitive(c.RootEmail, email)
}

// PrimaryRootEmail returns the first configured root email, or "" if none.
func (c *Config) PrimaryRootEmail() string { return PrimaryRootEmail(c.RootEmail) }

// RootEmails returns the configured root emails as a parsed list.
func (c *Config) RootEmails() []string { return ParseRootEmails(c.RootEmail) }
