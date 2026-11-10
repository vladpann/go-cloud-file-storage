package core_http_v1

import (
	"github.com/go-playground/validator/v10"
	core_http_v1_users "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/v1/users"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

var ApiVersion = "v1"

// V1 aggregates all v1 transport handlers
type V1 struct {
	Users     *core_http_v1_users.UsersTransport
	l         logger.Interface
	validator *validator.Validate
}

// NewV1 creates new V1 transport group
func NewV1(
	usersTransport *core_http_v1_users.UsersTransport,
	l logger.Interface,
) *V1 {
	return &V1{
		Users:     usersTransport,
		l:         l,
		validator: validator.New(validator.WithRequiredStructEnabled()),
	}
}

// Validator returns the shared validator instance
func (v *V1) Validator() *validator.Validate {
	return v.validator
}
