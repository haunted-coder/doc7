package doc7

import (
	"errors"
	"fmt"

	"github.com/magicrew/doc7/internal/vlm"
)

// ErrorKind identifies a stable class of conversion failure.
type ErrorKind string

const (
	ConfigError     ErrorKind = "ConfigError"
	DependencyError ErrorKind = "DependencyError"
	RenderError     ErrorKind = "RenderError"
	RateLimitError  ErrorKind = "RateLimitError"
	ServerError     ErrorKind = "ServerError"
	TimeoutError    ErrorKind = "TimeoutError"
	AuthError       ErrorKind = "AuthError"
	ModelError      ErrorKind = "ModelError"
	ParseError      ErrorKind = "ParseError"
	PartialError    ErrorKind = "PartialError"
)

// Error is the public, typed form of a doc7 conversion error.
type Error struct {
	Kind      ErrorKind `json:"kind"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
	Cause     error     `json:"-"`
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Kind, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Kind, e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func publicError(err error) error {
	if err == nil {
		return nil
	}
	var appErr *vlm.AppError
	if !errors.As(err, &appErr) {
		return err
	}
	return &Error{
		Kind:      ErrorKind(appErr.Kind),
		Message:   appErr.Message,
		Retryable: appErr.Retryable,
		Cause:     appErr.Cause,
	}
}
