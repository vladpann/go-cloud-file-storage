package user_service

import (
	"context"

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

type UsersRepository interface {
	SignUp(
		ctx context.Context,
		username string,
		passwordHash []byte,
	) (domain.User, error)
	GetUserByUsername(
		ctx context.Context,
		username string,
	) (domain.User, error)
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
