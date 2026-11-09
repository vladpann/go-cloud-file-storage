package core_http

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vladpann/go-cloud-file-storage/config"
	core_http_middleware "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/middleware"
	v1 "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/v1"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func NewRouter(r chi.Router, cfg *config.Config, l logger.Interface) {
	if cfg.Tracing.Enabled {
		r.Use(func(next http.Handler) http.Handler {
			return otelhttp.NewHandler(next, "http",
				otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
					return r.Method + " " + r.URL.Path
				}),

				otelhttp.WithFilter(func(r *http.Request) bool {
					return r.URL.Path != "/healthz"
				}),
			)
		})
		r.Use(core_http_middleware.TracingSpanName())
	}

	// Options
	r.Use(core_http_middleware.Recovery(l))
	r.Use(core_http_middleware.Logger(l))

	// K8s probe
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Routers
	r.Route(fmt.Sprintf("/api/%s", v1.ApiVersion), func(r chi.Router) {
		v1.NewRoutes(r, l)
	})
}
