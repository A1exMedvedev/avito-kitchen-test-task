package grpc

import (
	"context"
	"log/slog"
	"time"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const APIKeyMetadataKey = "x-api-key"

type venueContextKey struct{}

var codeForKind = map[domain.Kind]codes.Code{
	domain.KindInvalidArgument: codes.InvalidArgument,
	domain.KindUnauthenticated: codes.Unauthenticated,
	domain.KindForbidden:       codes.PermissionDenied,
	domain.KindNotFound:        codes.NotFound,
	domain.KindConflict:        codes.FailedPrecondition,
	domain.KindUnprocessable:   codes.FailedPrecondition,
	domain.KindInternal:        codes.Internal,
}

type Interceptors interface {
	Unary() grpc.UnaryServerInterceptor
	Stream() grpc.StreamServerInterceptor
}

type interceptors struct {
	partners PartnerUseCase
	logger   *slog.Logger
}

func NewInterceptors(partners PartnerUseCase, logger *slog.Logger) Interceptors {
	return &interceptors{partners: partners, logger: logger}
}

func (i *interceptors) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		started := time.Now()

		authenticated, err := i.authenticate(ctx)
		if err != nil {
			return nil, toStatusError(err)
		}
		resp, err := handler(authenticated, req)
		i.logger.Info("grpc call",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", time.Since(started)),
			slog.String("code", status.Code(toStatusError(err)).String()))
		if err != nil {
			return nil, toStatusError(err)
		}
		return resp, nil
	}
}

func (i *interceptors) Stream() grpc.StreamServerInterceptor {
	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		authenticated, err := i.authenticate(stream.Context())
		if err != nil {
			return toStatusError(err)
		}

		i.logger.Info("grpc stream opened", slog.String("method", info.FullMethod))
		err = handler(srv, &contextStream{ServerStream: stream, ctx: authenticated})
		i.logger.Info("grpc stream closed",
			slog.String("method", info.FullMethod),
			slog.String("code", status.Code(toStatusError(err)).String()))
		return toStatusError(err)
	}
}

func (i *interceptors) authenticate(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, domain.Unauthenticated("api_key_required",
			"%s metadata is required", APIKeyMetadataKey)
	}
	values := md.Get(APIKeyMetadataKey)
	if len(values) == 0 {
		return nil, domain.Unauthenticated("api_key_required",
			"%s metadata is required", APIKeyMetadataKey)
	}

	venue, err := i.partners.Authenticate(ctx, values[0])
	if err != nil {
		return nil, err
	}
	return context.WithValue(ctx, venueContextKey{}, venue), nil
}

func venueFrom(ctx context.Context) (*domain.Venue, error) {
	venue, ok := ctx.Value(venueContextKey{}).(*domain.Venue)
	if !ok {
		return nil, domain.Unauthenticated("api_key_required", "request is not authenticated")
	}
	return venue, nil
}

type contextStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *contextStream) Context() context.Context { return s.ctx }

func toStatusError(err error) error {
	if err == nil {
		return nil
	}
	if _, alreadyStatus := status.FromError(err); alreadyStatus && status.Code(err) != codes.Unknown {
		return err
	}

	domainErr, ok := domain.AsError(err)
	if !ok {
		return status.Error(codes.Internal, "internal error")
	}
	code, known := codeForKind[domainErr.Kind]
	if !known {
		code = codes.Internal
	}
	if code == codes.Internal {
		return status.Error(codes.Internal, "internal error")
	}
	return status.Errorf(code, "%s: %s", domainErr.Code, domainErr.Message)
}
