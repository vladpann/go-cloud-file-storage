package sessions_service

import (
	"context"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"

	"github.com/vladpann/go-cloud-file-storage/config"
	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
)

var tracer = otel.Tracer("github.com/vladpann/go-cloud-file-storage/internal/features/sessions/service")

type SessionsService struct {
	config             *config.Config
	sessionsRepository SessionsRepository
}

type SessionsRepository interface {
	CreateSession(
		ctx context.Context,
		session domain.Session,
	) (domain.Session, error)

	GetSession(
		ctx context.Context,
		sessionID uuid.UUID,
	) (domain.Session, error)

	DeleteSession(
		ctx context.Context,
		sessionID uuid.UUID,
	) error
}

func New(
	cfg *config.Config,
	sessionsRepository SessionsRepository,
) *SessionsService {
	return &SessionsService{
		config:             cfg,
		sessionsRepository: sessionsRepository,
	}
}
