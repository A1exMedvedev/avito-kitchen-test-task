package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/api/gen/openapi/kitchenapi"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/presentation/http/middleware"
)

var statusForKind = map[domain.Kind]int{
	domain.KindInvalidArgument: http.StatusBadRequest,
	domain.KindUnauthenticated: http.StatusUnauthorized,
	domain.KindForbidden:       http.StatusForbidden,
	domain.KindNotFound:        http.StatusNotFound,
	domain.KindConflict:        http.StatusConflict,
	domain.KindUnprocessable:   http.StatusUnprocessableEntity,
	domain.KindInternal:        http.StatusInternalServerError,
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		middleware.LoggerFrom(r.Context()).Error("write response body",
			slog.Any("error", err))
	}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	logger := middleware.LoggerFrom(r.Context())
	requestID := middleware.RequestIDFrom(r.Context())

	domainErr, ok := domain.AsError(err)
	if !ok {
		logger.Error("unhandled error", slog.Any("error", err))
		writeJSON(w, r, http.StatusInternalServerError, kitchenapi.Error{
			Code:      "internal_error",
			Message:   "internal error",
			RequestId: optional(requestID),
		})
		return
	}

	status, known := statusForKind[domainErr.Kind]
	if !known {
		status = http.StatusInternalServerError
	}
	if status >= http.StatusInternalServerError {
		logger.Error("request failed",
			slog.String("code", domainErr.Code),
			slog.Any("error", err))
	} else {
		logger.Info("request rejected",
			slog.Int("status", status),
			slog.String("code", domainErr.Code),
			slog.String("message", domainErr.Message))
	}

	body := kitchenapi.Error{
		Code:      domainErr.Code,
		Message:   domainErr.Message,
		RequestId: optional(requestID),
	}
	if len(domainErr.Items) > 0 {
		body.UnavailableItems = new(unavailableItemsToAPI(domainErr.Items))
	}
	writeJSON(w, r, status, body)
}

func unavailableItemsToAPI(items []domain.UnavailableItem) []kitchenapi.UnavailableItem {
	apiItems := make([]kitchenapi.UnavailableItem, 0, len(items))
	for _, item := range items {
		apiItems = append(apiItems, kitchenapi.UnavailableItem{
			MenuItemId: item.MenuItemID,
			Name:       item.Name,
			Requested:  item.Requested,
			Available:  item.Available,
			Reason:     kitchenapi.UnavailableItemReason(item.Reason),
		})
	}
	return apiItems
}

func decodeBody(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.Invalid("malformed_body", "request body is not valid JSON: %v", err)
	}
	return nil
}
