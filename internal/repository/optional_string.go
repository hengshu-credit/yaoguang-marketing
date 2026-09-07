package repository

import "database/sql"

// optionalString scans an optional legacy projection while keeping the public
// response's existing empty-string representation.
type optionalString struct{ value *string }

func (s optionalString) Scan(value any) error {
	var nullable sql.NullString
	if err := nullable.Scan(value); err != nil {
		return err
	}
	*s.value = nullable.String
	return nil
}
