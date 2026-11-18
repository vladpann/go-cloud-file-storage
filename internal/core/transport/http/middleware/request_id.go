package core_http_middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const (
	RequestIDHeader                = "X-Request-ID"
	requestIDContextKey contextKey = "request_id"
)

// RequestID middleware adds a unique request ID to each request
// It checks for existing X-Request-ID header, otherwise generates a new UUID
// The request ID is added to both the response header and the zerolog context
func RequestID() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(RequestIDHeader)
			if requestID == "" {
				requestID = uuid.New().String()
			}

			w.Header().Set(RequestIDHeader, requestID)

			ctx := context.WithValue(r.Context(), requestIDContextKey, requestID)

			logger := zerolog.Ctx(ctx).With().Str("request_id", requestID).Logger()
			ctx = logger.WithContext(ctx)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value(requestIDContextKey).(string); ok {
		return reqID
	}
	return ""
}
