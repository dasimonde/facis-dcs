package datatype

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
)

// JSON is a custom type for JSONB in PostgreSQL
type JSON json.RawMessage

// Value implements driver.Valuer for PostgreSQL
func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return json.RawMessage(j).MarshalJSON()
}

// Scan implements sql.Scanner for PostgreSQL
func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to unmarshal JSON value")
	}

	result := json.RawMessage{}
	err := result.UnmarshalJSON(bytes)
	*j = JSON(result)
	return err
}

// MarshalJSON implements json.Marshaler
func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return json.RawMessage(j).MarshalJSON()
}

// UnmarshalJSON implements json.Unmarshaler
func (j *JSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return errors.New("JSON: UnmarshalJSON on nil pointer")
	}
	*j = append((*j)[0:0], data...)
	return nil
}

func (j *JSON) IsNotNullValue() bool {

	if j == nil {
		return false
	}

	if len(*j) == 0 {
		return false
	}

	var value interface{}
	err := json.Unmarshal(*j, &value)
	if err != nil {
		return false
	}

	if value == nil {
		return false
	}

	return true
}

// NewJSON creates a JSON-Type from any Go value
func NewJSON(v any) (JSON, error) {
	if v == nil {
		return JSON("null"), nil
	}

	bytes, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal value into JSON: %w", err)
	}

	return JSON(bytes), nil
}
