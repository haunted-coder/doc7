package vlm

import (
	"errors"
	"fmt"
)

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

type AppError struct {
	Kind      ErrorKind `json:"kind"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
	Usage     *Usage    `json:"usage,omitempty"`
	Cause     error     `json:"-"`
}

func (e *AppError) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Kind, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Kind, e.Message, e.Cause)
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func NewError(kind ErrorKind, message string, retryable bool, cause error) *AppError {
	return &AppError{Kind: kind, Message: message, Retryable: retryable, Cause: cause}
}

func UsageFromError(err error) (Usage, bool) {
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Usage == nil {
		return Usage{}, false
	}
	return *appErr.Usage, true
}
