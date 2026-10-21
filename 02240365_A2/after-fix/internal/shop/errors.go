// Package shop holds the pure business rules of the five Part 2 services
// (cart, review, shipping, coupon, order history). It uses only the Go
// standard library, so every rule can be unit-tested with plain
// `go test ./internal/shop/` - no gRPC, no network, no generated code.
//
// Each service's main.go is a thin gRPC wrapper: it converts the protobuf
// request into a call on this package and converts the returned *Error
// into the matching gRPC status code (see internal/rpcerr).
package shop

import "fmt"

// Kind says what sort of failure an *Error is. The gRPC wrapper maps it to
// a status code the gateway already understands.
type Kind int

const (
	KindInvalid  Kind = iota + 1 // bad input            -> codes.InvalidArgument
	KindNotFound                 // no such record       -> codes.NotFound
	KindConflict                 // rule not satisfied   -> codes.FailedPrecondition
	KindExists                   // already exists       -> codes.AlreadyExists
)

// Error is a business-rule failure with a message that is safe to show to
// the person using the storefront.
type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func invalid(format string, a ...any) *Error  { return &Error{KindInvalid, fmt.Sprintf(format, a...)} }
func notFound(format string, a ...any) *Error { return &Error{KindNotFound, fmt.Sprintf(format, a...)} }
func conflict(format string, a ...any) *Error { return &Error{KindConflict, fmt.Sprintf(format, a...)} }
func exists(format string, a ...any) *Error   { return &Error{KindExists, fmt.Sprintf(format, a...)} }
