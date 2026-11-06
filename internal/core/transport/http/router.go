package core_http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vladpann/go-cloud-file-storage/config"
	v1 "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/v1"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

func NewRouter(r chi.Router, cfg *config.Config, l logger.Interface) {
	// Options
	//router.Use(middleware.Logger())

	// K8s probe
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Routers
	r.Route("/v1", func(r chi.Router) {
		v1.NewRoutes(r, l)
	})
}
