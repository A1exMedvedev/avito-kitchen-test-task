package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/api/gen/openapi/kitchenapi"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/presentation/http/middleware"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

const APIBasePath = "/api/v1"

type RouterOptions struct {
	Logger *slog.Logger

	ReadinessCheck func(r *http.Request) error

	Version string
}

func NewRouter(handler kitchenapi.ServerInterface, partners PartnerUseCase, opts RouterOptions) (http.Handler, error) {
	spec, err := kitchenapi.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("load embedded openapi spec: %w", err)
	}

	specRouter, err := gorillamux.NewRouter(spec)
	if err != nil {
		return nil, fmt.Errorf("build spec router: %w", err)
	}

	apiMux := http.NewServeMux()
	kitchenapi.HandlerWithOptions(handler, kitchenapi.StdHTTPServerOptions{
		BaseURL:    APIBasePath,
		BaseRouter: apiMux,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {

			writeError(w, r, domain.Invalid("invalid_parameter", "%v", err))
		},
	})

	validator := nethttpmiddleware.OapiRequestValidatorWithOptions(spec,
		&nethttpmiddleware.Options{

			SilenceServersWarning: true,

			Options: openapi3filter.Options{

				AuthenticationFunc: func(_ context.Context, _ *openapi3filter.AuthenticationInput) error {
					return nil
				},
			},
			ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, _ nethttpmiddleware.ErrorHandlerOpts) {
				writeError(w, r, schemaError(err))
			},
		})

	auth := newAuthenticator(specRouter, partners)

	root := http.NewServeMux()
	root.Handle(APIBasePath+"/", middleware.Chain(apiMux, validator, auth.Middleware))
	registerOpsRoutes(root, spec, opts)

	return middleware.Chain(root,
		middleware.Recover(),
		middleware.RequestID(opts.Logger),
		middleware.Logging(),
		middleware.CORS(),
	), nil
}

func schemaError(err error) error {
	var requestErr *openapi3filter.RequestError
	if errors.As(err, &requestErr) {
		return domain.Invalid("request_invalid", "%s", requestErr.Error())
	}

	var routeErr *routers.RouteError
	if errors.As(err, &routeErr) {
		return domain.NotFound("endpoint_not_found", "%s", routeErr.Error())
	}
	return domain.Invalid("request_invalid", "%v", err)
}

func registerOpsRoutes(mux *http.ServeMux, spec *openapi3.T, opts RouterOptions) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": opts.Version,
		})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if opts.ReadinessCheck != nil {
			if err := opts.ReadinessCheck(r); err != nil {
				middleware.LoggerFrom(r.Context()).Warn("readiness check failed",
					slog.Any("error", err))
				writeJSON(w, r, http.StatusServiceUnavailable,
					map[string]string{"status": "unavailable"})
				return
			}
		}
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "ready"})
	})

	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusOK, spec)
	})
}
