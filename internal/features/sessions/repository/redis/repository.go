package sessions_repository_redis

import (
	"github.com/google/uuid"
	"github.com/vladpann/go-cloud-file-storage/pkg/redis"
)

const keyPrefix = "session:"

type SessionsRepository struct {
	rdb *redis.Redis
}

func New(
	rdb *redis.Redis,
) *SessionsRepository {
	return &SessionsRepository{
		rdb: rdb,
	}
}

func sessionKey(id uuid.UUID) string {
	return keyPrefix + id.String()
}
