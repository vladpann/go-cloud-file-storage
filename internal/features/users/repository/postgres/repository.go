package users_repository_postgres

import (
	pkgpg "github.com/vladpann/go-cloud-file-storage/pkg/postgres"
)

type UsersRepository struct {
	pg *pkgpg.Postgres
}

func New(
	pg *pkgpg.Postgres,
) *UsersRepository {
	return &UsersRepository{
		pg: pg,
	}
}
