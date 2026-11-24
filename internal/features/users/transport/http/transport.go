package users_transport_http

import (
	"context"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"

	"github.com/vladpann/go-cloud-file-storage/config"
	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	core_http_middleware "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/middleware"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

var tracer = otel.Tracer("github.com/vladpann/go-cloud-file-storage/internal/features/users/transport/http")

// UsersHTTPHandler handles HTTP requests for user operations
type UsersHTTPHandler struct {
	usersService     UsersService
	sessionsService  SessionsService
	resourcesService ResourcesService
	authMiddleware   core_http_middleware.Middleware
	config           *config.Config
	l                logger.Interface
	validator        *validator.Validate
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

	GetUserByUsername(
		ctx context.Context,
		username string,
	) (domain.User, error)

	GetUsernameByUserID(
		ctx context.Context,
		userID uuid.UUID,
	) (string, error)
}

type SessionsService interface {
	CreateSession(
		ctx context.Context,
		userID uuid.UUID,
	) (domain.Session, error)
	DeleteSession(
		ctx context.Context,
		sessionID uuid.UUID,
	) error
}

type ResourcesService interface {
	CreateParentFolder(
		ctx context.Context,
		userID uuid.UUID,
	) error
}

func NewUsersHTTPHandler(
	usersService UsersService,
	sessionsService SessionsService,
	resourcesService ResourcesService,
	authMiddleware core_http_middleware.Middleware,
	cfg *config.Config,
	l logger.Interface,
	v *validator.Validate,
) *UsersHTTPHandler {
	return &UsersHTTPHandler{
		usersService:     usersService,
		sessionsService:  sessionsService,
		resourcesService: resourcesService,
		authMiddleware:   authMiddleware,
		config:           cfg,
		l:                l,
		validator:        v,
	}
}

// AuthMiddleware returns the authentication middleware
func (h *UsersHTTPHandler) AuthMiddleware() func(http.Handler) http.Handler {
	return h.authMiddleware
}
