package kimi

import "fmt"

type Error struct {
	Code      string
	Message   string
	Retryable bool
	Details   any
}

func (e *Error) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func IsCode(err error, code string) bool {
	if sdkErr, ok := err.(*Error); ok {
		return sdkErr.Code == code
	}
	return false
}

func protocolError(message string, code string, retryable bool, details any) *Error {
	return &Error{Code: code, Message: message, Retryable: retryable, Details: details}
}

func transportError(message string, details any) *Error {
	return &Error{Code: "TRANSPORT_ERROR", Message: message, Retryable: true, Details: details}
}
