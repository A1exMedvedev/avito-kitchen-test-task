package http

import (
	"context"
	"net/http"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers"
	"github.com/google/uuid"
)

type principalKey int

const (
	customerKey principalKey = iota
	venueKey
)

const (
	customerHeader = "X-Customer-Id"

	apiKeyHeader = "X-Api-Key" //nolint:gosec
)

const (
	schemeCustomer = "customerAuth"
	schemePartner  = "partnerAuth"
)

func CustomerFrom(ctx context.Context) (uuid.UUID, error) {
	id, ok := ctx.Value(customerKey).(uuid.UUID)
	if !ok {
		return uuid.Nil, domain.Unauthenticated("customer_required",
			"X-Customer-Id header is required")
	}
	return id, nil
}

func VenueFrom(ctx context.Context) (*domain.Venue, error) {
	venue, ok := ctx.Value(venueKey).(*domain.Venue)
	if !ok {
		return nil, domain.Unauthenticated("api_key_required", "X-Api-Key header is required")
	}
	return venue, nil
}

type authenticator struct {
	router   routers.Router
	partners PartnerUseCase
}

func newAuthenticator(router routers.Router, partners PartnerUseCase) *authenticator {
	return &authenticator{router: router, partners: partners}
}

func (a *authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, _, err := a.router.FindRoute(r)
		if err != nil {

			next.ServeHTTP(w, r)
			return
		}

		requirements := route.Operation.Security
		if requirements == nil {

			requirements = &route.Spec.Security
		}
		if requirements == nil || len(*requirements) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		ctx, err := a.authenticate(r.Context(), r, *requirements)
		if err != nil {
			writeError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *authenticator) authenticate(
	ctx context.Context,
	r *http.Request,
	requirements openapi3.SecurityRequirements,
) (context.Context, error) {
	var lastErr error

	for _, requirement := range requirements {
		candidate := ctx
		satisfied := true

		for scheme := range requirement {
			next, err := a.applyScheme(candidate, r, scheme)
			if err != nil {
				lastErr = err
				satisfied = false
				break
			}
			candidate = next
		}
		if satisfied {
			return candidate, nil
		}
	}

	if lastErr == nil {
		lastErr = domain.Unauthenticated("unauthenticated", "credentials are required")
	}
	return ctx, lastErr
}

func (a *authenticator) applyScheme(ctx context.Context, r *http.Request, scheme string) (context.Context, error) {
	switch scheme {
	case schemeCustomer:
		raw := r.Header.Get(customerHeader)
		if raw == "" {
			return ctx, domain.Unauthenticated("customer_required",
				"%s header is required", customerHeader)
		}

		customerID, err := uuid.Parse(raw)
		if err != nil {
			return ctx, domain.Unauthenticated("customer_invalid",
				"%s must be a UUID", customerHeader)
		}
		return context.WithValue(ctx, customerKey, customerID), nil

	case schemePartner:
		venue, err := a.partners.Authenticate(ctx, r.Header.Get(apiKeyHeader))
		if err != nil {
			return ctx, err
		}
		return context.WithValue(ctx, venueKey, venue), nil

	default:

		return ctx, domain.Internal(
			domain.Invalid("unknown_security_scheme", "unsupported security scheme %q", scheme))
	}
}
