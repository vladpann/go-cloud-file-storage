package sessions_repository_redis

import (
	"github.com/google/uuid"

	pkgredis "github.com/vladpann/go-cloud-file-storage/pkg/redis"
)

const keyPrefix = "session:"

type SessionsRepository struct {
	rdb *pkgredis.Redis
}

func New(
	rdb *pkgredis.Redis,
) *SessionsRepository {
	return &SessionsRepository{
		rdb: rdb,
	}
}

func sessionKey(id uuid.UUID) string {
	return keyPrefix + id.String()
}
