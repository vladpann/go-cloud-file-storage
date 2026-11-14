package sessions_service

import (
	"context"

	"github.com/google/uuid"
)

func (s *SessionsService) DeleteSession(
	ctx context.Context,
	sessionID uuid.UUID,
) error {
	return s.sessionsRepository.DeleteSession(ctx, sessionID)
}
