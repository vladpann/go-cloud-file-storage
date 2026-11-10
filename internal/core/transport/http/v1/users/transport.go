package core_http_v1_users

import (
	"context"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	core_http_middleware "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/middleware"
	"github.com/vladpann/go-cloud-file-storage/internal/domain"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

// UsersTransport handles HTTP requests for user operations
type UsersTransport struct {
	usersService    UsersService
	sessionsService SessionsService
	authMiddleware  core_http_middleware.Middleware
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

func NewUsersTransport(
	usersService UsersService,
	sessionsService SessionsService,
	authMiddleware core_http_middleware.Middleware,
	l logger.Interface,
	validator *validator.Validate,
) *UsersTransport {
	return &UsersTransport{
		usersService:    usersService,
		sessionsService: sessionsService,
		authMiddleware:  authMiddleware,
		l:               l,
		validator:       validator,
	}
}
