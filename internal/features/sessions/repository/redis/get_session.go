package sessions_repository_redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	core_errors "github.com/vladpann/go-cloud-file-storage/internal/core/errors"
	"github.com/vladpann/go-cloud-file-storage/pkg/redis"
)

func (r *SessionsRepository) GetSession(
	ctx context.Context,
	sessionID uuid.UUID,
) (domain.Session, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	data, err := r.rdb.Client.Get(ctx, sessionKey(sessionID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.ErrNil) {
			return domain.Session{}, fmt.Errorf("session %s: %w", sessionID, core_errors.ErrNotFound)
		}

		return domain.Session{}, fmt.Errorf("redis get session: %w", err)
	}

	var model SessionModel
	if err := json.Unmarshal(data, &model); err != nil {
		return domain.Session{}, fmt.Errorf("unmarshal session: %w", err)
	}

	sessionDomain := sessionDomainFromModel(model)

	return sessionDomain, nil
}
