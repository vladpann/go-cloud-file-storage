package users_repository_postgres

import (
	"go.opentelemetry.io/otel"

	pkgpg "github.com/vladpann/go-cloud-file-storage/pkg/postgres"
)

var tracer = otel.Tracer("github.com/vladpann/go-cloud-file-storage/internal/features/users/repository/postgres")

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
