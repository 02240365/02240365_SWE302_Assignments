// Package rpcerr converts a shop business-rule error into a gRPC status
// error, so the gateway's existing error handling treats it like any
// other service.
package rpcerr

import (
	"errors"

	"ecommerce-microservices/internal/shop"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// To returns nil for nil; a *shop.Error becomes the matching gRPC status;
// anything else becomes Internal.
func To(err error) error {
	if err == nil {
		return nil
	}
	var se *shop.Error
	if errors.As(err, &se) {
		switch se.Kind {
		case shop.KindInvalid:
			return status.Error(codes.InvalidArgument, se.Msg)
		case shop.KindNotFound:
			return status.Error(codes.NotFound, se.Msg)
		case shop.KindConflict:
			return status.Error(codes.FailedPrecondition, se.Msg)
		case shop.KindExists:
			return status.Error(codes.AlreadyExists, se.Msg)
		}
	}
	return status.Error(codes.Internal, "internal error")
}
