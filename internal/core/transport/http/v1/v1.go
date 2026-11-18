package core_http_v1

import (
	"github.com/go-playground/validator/v10"

	users_transport_http "github.com/vladpann/go-cloud-file-storage/internal/features/users/transport/http"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

var ApiVersion = "v1"

// V1 aggregates all v1 transport
type V1 struct {
	Users     *users_transport_http.UsersHTTPHandler
	l         logger.Interface
	validator *validator.Validate
}

// NewV1 creates new V1 transport group
func NewV1(
	usersHTTPHandler *users_transport_http.UsersHTTPHandler,
	l logger.Interface,
) *V1 {
	return &V1{
		Users:     usersHTTPHandler,
		l:         l,
		validator: validator.New(validator.WithRequiredStructEnabled()),
	}
}

// Validator returns the shared validator instance
func (v *V1) Validator() *validator.Validate {
	return v.validator
}
