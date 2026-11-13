package users_transport_http

import (
	"context"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/vladpann/go-cloud-file-storage/config"
	core_http_middleware "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/middleware"
	"github.com/vladpann/go-cloud-file-storage/internal/domain"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

// UsersHTTPHandler handles HTTP requests for user operations
type UsersHTTPHandler struct {
	usersService    UsersService
	sessionsService SessionsService
	authMiddleware  core_http_middleware.Middleware
	config          *config.Config
	l               logger.Interface
	validator       *validator.Validate
}

type UsersService interface {
	SignUp(
		ctx context.Context,
		username string,
		password string,
		repeatPassword string,
	) (domain.User, error)
	SignIn(
		ctx context.Context,
		username string,
		password string,
	) (uuid.UUID, error)
}

type SessionsService interface {
	CreateSession(
		ctx context.Context,
		userID uuid.UUID,
	) (domain.Session, error)
}

func NewUsersHTTPHandler(
	usersService UsersService,
	sessionsService SessionsService,
	authMiddleware core_http_middleware.Middleware,
	config *config.Config,
	l logger.Interface,
	validator *validator.Validate,
) *UsersHTTPHandler {
	return &UsersHTTPHandler{
		usersService:    usersService,
		sessionsService: sessionsService,
		authMiddleware:  authMiddleware,
		config:          config,
		l:               l,
		validator:       validator,
	}
}
