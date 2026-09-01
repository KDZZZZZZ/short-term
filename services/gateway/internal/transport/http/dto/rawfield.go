package dto

import (
	"bytes"
	"encoding/json"
	"errors"
)

// RawField distinguishes the three states a JSON property can be in: absent,
// present with a value, or present and null. encoding/json cannot express that
// with a plain pointer, and the contract's PATCH bodies depend on it.
type RawField struct {
	// Present is true when the property appeared in the request body.
	Present bool
	raw     json.RawMessage
}

// ErrFieldNotString reports a field whose value is neither a string nor null.
var ErrFieldNotString = errors.New("field must be a string or null")

// UnmarshalJSON records the raw value and marks the field present.
func (f *RawField) UnmarshalJSON(data []byte) error {
	f.Present = true
	f.raw = append(f.raw[:0], data...)
	return nil
}

// MarshalJSON renders the field as it arrived, so a RawField can round-trip.
func (f RawField) MarshalJSON() ([]byte, error) {
	if !f.Present || len(f.raw) == 0 {
		return []byte("null"), nil
	}
	return f.raw, nil
}

// IsNull reports whether the property was present and explicitly null.
func (f RawField) IsNull() bool {
	return f.Present && bytes.Equal(bytes.TrimSpace(f.raw), []byte("null"))
}

// String decodes a present, non-null field as a string.
func (f RawField) String() (string, error) {
	var value string
	if err := json.Unmarshal(f.raw, &value); err != nil {
		return "", ErrFieldNotString
	}
	return value, nil
}
