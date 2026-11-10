package user_service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vladpann/go-cloud-file-storage/internal/domain"
)

type PasswordHasher interface {
	Hash(password string) ([]byte, error)
	Compare(hash []byte, password string) error
}

type UsersService struct {
	usersRepository UsersRepository
	passwordHasher  PasswordHasher
}

type UserModel struct {
	ID           uuid.UUID
	Username     string
	PasswordHash []byte
}

type UsersRepository interface {
	SignUp(
		ctx context.Context,
		username string,
		passwordHash []byte,
	) (domain.User, error)
	GetUserByUsername(
		ctx context.Context,
		username string,
	) (UserModel, error)
}

func New(
	usersRepository UsersRepository,
	passwordHasher PasswordHasher,
) *UsersService {
	return &UsersService{
		usersRepository: usersRepository,
		passwordHasher:  passwordHasher,
	}
}
