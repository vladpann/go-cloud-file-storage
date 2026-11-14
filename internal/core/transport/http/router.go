package core_http

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vladpann/go-cloud-file-storage/config"
	core_http_middleware "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/middleware"
	v1 "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/v1"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

func NewRouter(r chi.Router, cfg *config.Config, v1Transports *v1.V1, l logger.Interface) {
	// Build middleware chain
	middlewares := buildMiddlewareChain(cfg, l)

	// Apply all middlewares
	for _, mw := range middlewares {
		r.Use(mw)
	}

	// K8s probe
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Routers
	r.Route(fmt.Sprintf("/api/%s", v1.ApiVersion), func(r chi.Router) {
		v1.NewRoutes(r, v1Transports, l)
	})
}

// buildMiddlewareChain constructs the middleware chain in execution order
func buildMiddlewareChain(cfg *config.Config, l logger.Interface) []core_http_middleware.Middleware {
	middlewares := []core_http_middleware.Middleware{
		// 1. Request ID - must be first to ensure all logs and traces have request_id
		core_http_middleware.RequestID(),
	}

	// 2. Tracing (if enabled)
	if cfg.Tracing.Enabled {
		middlewares = append(middlewares, core_http_middleware.TracingMiddlewares()...)
	}

	// 3. Recovery - should be early to catch panics from other middleware
	middlewares = append(middlewares, core_http_middleware.Recovery(l))

	// 4. Logger - after recovery to ensure panics are logged
	middlewares = append(middlewares, core_http_middleware.Logger(l))

	return middlewares
}
