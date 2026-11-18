package core_http_middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TracingMiddlewares returns a slice of tracing middlewares configured for the application.
// It includes OpenTelemetry HTTP instrumentation and custom span naming.
func TracingMiddlewares() []Middleware {
	return []Middleware{
		// OpenTelemetry HTTP instrumentation
		func(next http.Handler) http.Handler {
			return otelhttp.NewHandler(next, "http",
				otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
					return r.Method + " " + r.URL.Path
				}),
				otelhttp.WithFilter(func(r *http.Request) bool {
					// Skip tracing for health check endpoint
					return r.URL.Path != "/healthz"
				}),
			)
		},
		// Custom span naming and request_id attribute
		TracingSpanName(),
	}
}

// TracingSpanName renames the current span to 'METHOD /route/{pattern}'
// once chi has determined the route, and adds request_id as an attribute.
func TracingSpanName() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add request_id to the span if available
			if requestID := GetRequestID(r.Context()); requestID != "" {
				span := trace.SpanFromContext(r.Context())
				span.SetAttributes(attribute.String(string(requestIDContextKey), requestID))
			}

			next.ServeHTTP(w, r)

			rctx := chi.RouteContext(r.Context())
			if rctx == nil {
				return
			}

			pattern := rctx.RoutePattern()
			if pattern == "" {
				return
			}

			trace.SpanFromContext(r.Context()).SetName(r.Method + " " + pattern)
		})
	}
}
