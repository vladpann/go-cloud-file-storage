package domain

import (
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
}

func NewSession(
	id uuid.UUID,
	userID uuid.UUID,
	expiresAt time.Time,
) Session {
	return Session{
		ID:        id,
		UserID:    userID,
		ExpiresAt: expiresAt,
	}
}
