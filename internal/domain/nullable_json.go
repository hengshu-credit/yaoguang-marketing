package domain

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// NullableJSON represents a JSON object that can be null.
// It implements database/sql.Scanner and driver.Valuer interfaces
// for proper database handling, as well as json.Marshaler and
// json.Unmarshaler for JSON encoding/decoding.
type NullableJSON struct {
	Data   interface{}
	IsNull bool
}

// Scan implements the sql.Scanner interface.
// It scans a value from the database into the NullableJSON struct.
func (nj *NullableJSON) Scan(value interface{}) error {
	if value == nil {
		nj.Data = nil
		nj.IsNull = true
		return nil
	}

	// Handle byte slice from database
	switch v := value.(type) {
	case []byte:
		if len(v) == 0 {
			nj.Data = nil
			nj.IsNull = true
			return nil
		}
		// VERY IMPORTANT: we need to clone the bytes here
		// The sql driver will reuse the same bytes RAM slots for future queries
		// Thank you St Antoine De Padoue for helping me find this bug
		cloned := bytes.Clone(v)
		var data interface{}
		if err := json.Unmarshal(cloned, &data); err != nil {
			return err
		}
		nj.Data = data
		nj.IsNull = false
		return nil
	case string:
		if v == "" {
			nj.Data = nil
			nj.IsNull = true
			return nil
		}
		// VERY IMPORTANT: we need to clone the bytes here
		// The sql driver will reuse the same bytes RAM slots for future queries
		// Thank you St Antoine De Padoue for helping me find this bug
		cloned := bytes.Clone([]byte(v))
		var data interface{}
		if err := json.Unmarshal(cloned, &data); err != nil {
			return err
		}
		nj.Data = data
		nj.IsNull = false
		return nil
	default:
		return errors.New("incompatible type for NullableJSON")
	}
}

// Value implements the driver.Valuer interface.
// It returns a value suitable for database storage.
func (nj NullableJSON) Value() (driver.Value, error) {
	if nj.IsNull || nj.Data == nil {
		return nil, nil
	}

	return json.Marshal(nj.Data)
}

// MarshalJSON implements the json.Marshaler interface.
// It handles the JSON encoding of the NullableJSON value.
func (nj NullableJSON) MarshalJSON() ([]byte, error) {
	if nj.IsNull || nj.Data == nil {
		return []byte("null"), nil
	}
	return json.Marshal(nj.Data)
}

// UnmarshalJSON implements the json.Unmarshaler interface.
// It handles the JSON decoding into a NullableJSON value.
func (nj *NullableJSON) UnmarshalJSON(data []byte) error {
	// Handle null case
	if string(data) == "null" {
		nj.Data = nil
		nj.IsNull = true
		return nil
	}

	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	nj.Data = value
	nj.IsNull = false
	return nil
}
