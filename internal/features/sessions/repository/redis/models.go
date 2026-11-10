package sessions_repository_redis

import (
	"time"

	"github.com/google/uuid"
	"github.com/vladpann/go-cloud-file-storage/internal/domain"
)

type SessionModel struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
}

func sessionDomainFromModel(session SessionModel) domain.Session {
	return domain.NewSession(
		session.ID,
		session.UserID,
		session.ExpiresAt,
	)
}

func sessionModelFromDomain(session domain.Session) SessionModel {
	return SessionModel{
		ID:        session.ID,
		UserID:    session.UserID,
		ExpiresAt: session.ExpiresAt,
	}
}
