package core_http_v1

import (
	"github.com/go-chi/chi/v5"

	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

// NewRoutes registers all v1 API routes
func NewRoutes(apiV1Group chi.Router, v1Transports *V1, l logger.Interface) {
	// Auth routes
	apiV1Group.Route("/auth", func(r chi.Router) {
		// Public routes
		r.Post("/sign-up", v1Transports.Users.SignUp)
		r.Post("/sign-in", v1Transports.Users.SignIn)

		// Protected routes (require authentication)
		r.With(v1Transports.Users.AuthMiddleware()).Post("/sign-out", v1Transports.Users.SignOut)
	})
}
