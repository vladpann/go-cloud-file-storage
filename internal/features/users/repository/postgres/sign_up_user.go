package users_repository_postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	core_errors "github.com/vladpann/go-cloud-file-storage/internal/core/errors"
)

var tracer = otel.Tracer("github.com/vladpann/go-cloud-file-storage/internal/features/users/repository/postgres")

func (r *UsersRepository) SignUp(
	ctx context.Context,
	username string,
	passwordHash []byte,
) (domain.User, error) {
	ctx, span := tracer.Start(ctx, "UsersRepository.SignUp")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.username", username),
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.table", "cloud_file_storage.users"),
	)

	sql, args, err := r.pg.Builder.
		Insert("cloud_file_storage.users").
		Columns("username", "password_hash").
		Values(username, passwordHash).
		Suffix("RETURNING id, username, password_hash").
		ToSql()
	if err != nil {
		err = fmt.Errorf("users_repository_postgres - SignUp - r.pg.Builder: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to build SQL query")
		return domain.User{}, err
	}

	span.SetAttributes(attribute.String("db.statement", sql))
	span.AddEvent("executing SQL query")

	row := r.pg.Pool.QueryRow(ctx, sql, args...)

	var userModel UserModel
	err = row.Scan(
		&userModel.ID,
		&userModel.Username,
		&userModel.PasswordHash,
	)
	if err != nil {
		// Check for unique constraint violation (duplicate username)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			err = fmt.Errorf("users_repository_postgres - SignUp: %w", core_errors.ErrUserAlreadyExists)
			span.RecordError(err)
			span.SetStatus(codes.Error, "user already exists")
			span.SetAttributes(attribute.String("db.error.code", "23505"))
			return domain.User{}, err
		}

		if errors.Is(err, pgx.ErrNoRows) {
			err = fmt.Errorf("users_repository_postgres - SignUp - no rows returned: %w", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "no rows returned")
			return domain.User{}, err
		}

		err = fmt.Errorf("users_repository_postgres - SignUp - row.Scan: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to scan row")
		return domain.User{}, err
	}

	userDomain := userDomainFromModel(userModel)

	span.SetAttributes(attribute.String("user.id", userDomain.ID.String()))
	span.SetStatus(codes.Ok, "user inserted successfully")

	return userDomain, nil
}
