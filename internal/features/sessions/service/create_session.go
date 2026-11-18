package sessions_service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
)

var tracer = otel.Tracer("github.com/vladpann/go-cloud-file-storage/internal/features/sessions/service")

func (s *SessionsService) CreateSession(
	ctx context.Context,
	userID uuid.UUID,
) (domain.Session, error) {
	ctx, span := tracer.Start(ctx, "SessionsService.CreateSession")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", userID.String()))

	session := domain.Session{
		ID:        uuid.New(),
		UserID:    userID,
		ExpiresAt: time.Now().Add(s.config.Session.TTL),
	}

	span.SetAttributes(
		attribute.String("session.id", session.ID.String()),
		attribute.String("session.expires_at", session.ExpiresAt.Format(time.RFC3339)),
	)

	session, err := s.sessionsRepository.CreateSession(ctx, session)
	if err != nil {
		err = fmt.Errorf("sessions_service - CreateSession - sessionsRepository.CreateSession: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create session in repository")
		return domain.Session{}, err
	}

	span.SetStatus(codes.Ok, "session created successfully")

	return session, nil
}
