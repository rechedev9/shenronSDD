package errs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// JSONError is the structured error envelope written to stderr.
type JSONError struct {
	Command string `json:"command"`
	Error   string `json:"error"`
	Code    string `json:"code"` // "usage", "transport", "internal", "not_implemented"
}

// usageError marks errors caused by invalid CLI usage (exit code 2).
type usageError struct {
	msg string
}

func (e *usageError) Error() string { return e.msg }

// Usage returns a usage error (exit code 2).
func Usage(msg string) error {
	return &usageError{msg: msg}
}

// IsUsage reports whether err is a usage error.
func IsUsage(err error) bool {
	var ue *usageError
	return errors.As(err, &ue)
}

// transportError marks errors caused by network/external process failures.
// Used for network, git, and external process failures.
type transportError struct {
	msg string
}

func (e *transportError) Error() string { return e.msg }

// Transport returns a transport error (network, git, external process failures).
func Transport(msg string) error {
	return &transportError{msg: msg}
}

// IsTransport reports whether err is a transport error.
func IsTransport(err error) bool {
	var te *transportError
	return errors.As(err, &te)
}

// WriteJSON writes a structured JSON error to w and returns a generic error.
func WriteJSON(w io.Writer, command, message string) error {
	code := "not_implemented"
	je := JSONError{
		Command: command,
		Error:   message,
		Code:    code,
	}
	data, _ := json.Marshal(je)
	fmt.Fprintln(w, string(data))
	return fmt.Errorf("%s: %s", command, message)
}

// WriteError writes a structured JSON error to w for real failures.
func WriteError(w io.Writer, command string, err error) error {
	code := "internal"
	if IsUsage(err) {
		code = "usage"
	} else if IsTransport(err) {
		code = "transport"
	}
	je := JSONError{
		Command: command,
		Error:   err.Error(),
		Code:    code,
	}
	data, _ := json.Marshal(je)
	fmt.Fprintln(w, string(data))
	return err
}
