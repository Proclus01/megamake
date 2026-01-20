package errors

import (
	"errors"
	"fmt"
)

type Kind string

const (
	KindUsage    Kind = "usage"
	KindPolicy   Kind = "policy"
	KindIO       Kind = "io"
	KindExec     Kind = "exec"
	KindInternal Kind = "internal"
)

type AppError struct {
	Kind    Kind
	Message string
	Cause   error
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Kind, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Kind, e.Message, e.Cause)
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func New(kind Kind, message string, cause error) error {
	return &AppError{
		Kind:    kind,
		Message: message,
		Cause:   cause,
	}
}

func NewUsage(message string) error {
	return New(KindUsage, message, nil)
}

func NewPolicy(message string) error {
	return New(KindPolicy, message, nil)
}

func NewInternal(message string, cause error) error {
	return New(KindInternal, message, cause)
}

func IsUsage(err error) bool {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae.Kind == KindUsage
	}
	return false
}
