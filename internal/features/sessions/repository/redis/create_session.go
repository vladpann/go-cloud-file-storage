package sessions_repository_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
)

var tracer = otel.Tracer("github.com/vladpann/go-cloud-file-storage/internal/features/sessions/repository/redis")

func (r *SessionsRepository) CreateSession(
	ctx context.Context,
	session domain.Session,
) (domain.Session, error) {
	ctx, span := tracer.Start(ctx, "SessionsRepository.CreateSession")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.id", session.UserID.String()),
		attribute.String("db.system", "redis"),
		attribute.String("db.operation", "SET"),
	)

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		err := fmt.Errorf("session already expired")
		span.RecordError(err)
		span.SetStatus(codes.Error, "session expired")
		return domain.Session{}, err
	}

	span.SetAttributes(attribute.Int64("session.ttl_seconds", int64(ttl.Seconds())))

	sessionModel := sessionModelFromDomain(session)
	data, err := json.Marshal(sessionModel)
	if err != nil {
		err = fmt.Errorf("marshal session: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal session")
		return domain.Session{}, err
	}

	key := sessionKey(session.ID)
	span.AddEvent("setting session in Redis")

	if err := r.rdb.Client.Set(ctx, key, data, ttl).Err(); err != nil {
		err = fmt.Errorf("redis set session: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to set session in Redis")
		return domain.Session{}, err
	}

	span.SetStatus(codes.Ok, "session stored in Redis")

	return session, nil
}
