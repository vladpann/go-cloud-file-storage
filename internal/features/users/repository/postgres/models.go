package users_repository_postgres

import (
	"github.com/google/uuid"
	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
)

type UserModel struct {
	ID           uuid.UUID
	Username     string
	PasswordHash []byte
}

func userDomainFromModel(user UserModel) domain.User {
	return domain.User{
		ID:           user.ID,
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
	}
}
