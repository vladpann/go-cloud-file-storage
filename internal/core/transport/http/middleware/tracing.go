package core_http_middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"
)

// TracingSpanName renames the current span to ‘METHOD /route/{pattern}’
// once chi has determined the route.
func TracingSpanName() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
