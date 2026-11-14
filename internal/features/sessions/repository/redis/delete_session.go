package sessions_repository_redis

import (
	"context"

	"github.com/google/uuid"
)

func (r *SessionsRepository) DeleteSession(
	ctx context.Context,
	sessionID uuid.UUID,
) error {
	return r.rdb.Client.Del(ctx, sessionKey(sessionID)).Err()
}
