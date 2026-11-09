package catalog

import (
	"errors"
	"fmt"
)

// Sentinel errors identify failures callers can act on without parsing text.
var (
	// ErrInvalid reports a record that cannot be used.
	ErrInvalid = errors.New("invalid catalog record")
	// ErrUnsupportedScheme reports a non-HTTP stream or source URL.
	ErrUnsupportedScheme = errors.New("unsupported URL scheme")
	// ErrCredentials reports a URL that embeds a user name or password.
	ErrCredentials = errors.New("URL credentials are not supported")
	// ErrPrivateAddress reports a loopback or private network destination.
	ErrPrivateAddress = errors.New("private network address is not allowed")
)

// Code is a stable, machine-readable reason for a validation failure or a
// playlist warning. Callers present their own human text for each code.
type Code string

// Validation and warning codes.
const (
	CodeInvalidRecord     Code = "invalid_record"
	CodeInvalidURL        Code = "invalid_url"
	CodeUnsupportedScheme Code = "unsupported_scheme"
	CodeCredentials       Code = "credentials_unsupported"
	CodePrivateAddress    Code = "private_address"
	CodeMissingName       Code = "missing_name"
	CodeMissingStream     Code = "missing_stream"
	CodeInvalidHeader     Code = "invalid_header"
)

// ValidationError explains which field of a record is unusable.
type ValidationError struct {
	Field   string
	Code    Code
	Message string
	Err     error
}

// Error implements error.
func (e *ValidationError) Error() string {
	field := e.Field
	if field == "" {
		field = "record"
	}
	if e.Message != "" {
		return fmt.Sprintf("%s: %s (%s)", field, e.Message, e.Code)
	}

	return fmt.Sprintf("%s: %s", field, e.Code)
}

// Unwrap returns the underlying sentinel error.
func (e *ValidationError) Unwrap() error {
	return e.Err
}

func invalid(field string, code Code, err error, message string) error {
	return &ValidationError{
		Field:   field,
		Code:    code,
		Message: message,
		Err:     errors.Join(ErrInvalid, err),
	}
}
