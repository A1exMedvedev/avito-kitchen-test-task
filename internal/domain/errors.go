package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type Kind string

const (
	KindInvalidArgument Kind = "invalid_argument"
	KindNotFound        Kind = "not_found"
	KindConflict        Kind = "conflict"
	KindUnauthenticated Kind = "unauthenticated"
	KindForbidden       Kind = "forbidden"
	KindUnprocessable   Kind = "unprocessable"
	KindInternal        Kind = "internal"
)

type UnavailableReason string

const (
	ReasonSoldOut           UnavailableReason = "sold_out"
	ReasonInsufficientStock UnavailableReason = "insufficient_stock"
	ReasonDisabled          UnavailableReason = "disabled"
	ReasonWithdrawn         UnavailableReason = "withdrawn"
)

type UnavailableItem struct {
	MenuItemID uuid.UUID
	Name       string
	Requested  int
	Available  int
	Reason     UnavailableReason
}

type Error struct {
	Kind Kind

	Code string

	Message string

	Items []UnavailableItem

	cause error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s (%s): %s: %v", e.Code, e.Kind, e.Message, e.cause)
	}
	return fmt.Sprintf("%s (%s): %s", e.Code, e.Kind, e.Message)
}

func (e *Error) Unwrap() error { return e.cause }

func (e *Error) WithCause(cause error) *Error {
	clone := *e
	clone.cause = cause
	return &clone
}

func newError(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

func Invalid(code, format string, args ...any) *Error {
	return newError(KindInvalidArgument, code, fmt.Sprintf(format, args...))
}

func NotFound(code, format string, args ...any) *Error {
	return newError(KindNotFound, code, fmt.Sprintf(format, args...))
}

func Conflict(code, format string, args ...any) *Error {
	return newError(KindConflict, code, fmt.Sprintf(format, args...))
}

func Forbidden(code, format string, args ...any) *Error {
	return newError(KindForbidden, code, fmt.Sprintf(format, args...))
}

func Unauthenticated(code, format string, args ...any) *Error {
	return newError(KindUnauthenticated, code, fmt.Sprintf(format, args...))
}

func Unprocessable(code, format string, args ...any) *Error {
	return newError(KindUnprocessable, code, fmt.Sprintf(format, args...))
}

func Internal(cause error) *Error {
	return (&Error{
		Kind:    KindInternal,
		Code:    "internal_error",
		Message: "internal error",
	}).WithCause(cause)
}

func AsError(err error) (*Error, bool) {
	if domainErr, ok := errors.AsType[*Error](err); ok {
		return domainErr, true
	}
	return nil, false
}
