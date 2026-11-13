package users_repository_postgres

import (
	"github.com/vladpann/go-cloud-file-storage/pkg/postgres"
)

type UsersRepository struct {
	pg *postgres.Postgres
}

func New(
	pg *postgres.Postgres,
) *UsersRepository {
	return &UsersRepository{
		pg: pg,
	}
}
