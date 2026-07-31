package signingstatus

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

type SigningStatus string

const (
	Pending SigningStatus = "PENDING"
	Signed  SigningStatus = "SIGNED"
	Revoked SigningStatus = "REVOKED"
)

var validValues = map[SigningStatus]bool{
	Pending: true,
	Signed:  true,
	Revoked: true,
}

func NewSigningStatus(s string) (SigningStatus, error) {
	ts := SigningStatus(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid signing status: %s", s)
	}
	return ts, nil
}

// IsValid reports whether the value is one of the declared SigningStatus values.
func (s SigningStatus) IsValid() bool {
	upper := SigningStatus(strings.ToUpper(string(s)))
	return validValues[upper]
}

// String returns the string representation of the SigningStatus
func (s SigningStatus) String() string {
	return string(s)
}

// Scan implements the sql.Scanner interface
func (s *SigningStatus) Scan(value interface{}) error {
	if value == nil {
		return fmt.Errorf("signing status cannot be null")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("unsupported type for SigningStatus: %T", value)
	}

	status, err := NewSigningStatus(str)
	if err != nil {
		return err
	}

	*s = status
	return nil
}

// Value implements the driver.Valuer interface
func (s SigningStatus) Value() (driver.Value, error) {
	if !s.IsValid() {
		return nil, fmt.Errorf("invalid signing status: %s", s)
	}
	return string(s), nil
}
