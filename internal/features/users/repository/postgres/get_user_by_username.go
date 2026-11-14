package users_repository_postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	core_errors "github.com/vladpann/go-cloud-file-storage/internal/core/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func (r *UsersRepository) GetUserByUsername(
	ctx context.Context,
	username string,
) (domain.User, error) {
	ctx, span := tracer.Start(ctx, "UsersRepository.GetUserByUsername")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.username", username),
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.table", "cloud_file_storage.users"),
	)

	sql, args, err := r.pg.Builder.
		Select("id", "username", "password_hash").
		From("cloud_file_storage.users").
		Where("username = ?", username).
		ToSql()
	if err != nil {
		err = fmt.Errorf("users_repository_postgres - GetUserByUsername - r.pg.Builder: %w", err)
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
		if errors.Is(err, pgx.ErrNoRows) {
			err = fmt.Errorf("users_repository_postgres - GetUserByUsername: %w", core_errors.ErrNotFound)
			span.RecordError(err)
			span.SetStatus(codes.Error, "user not found")
			return domain.User{}, err
		}

		err = fmt.Errorf("users_repository_postgres - GetUserByUsername - row.Scan: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to scan row")
		return domain.User{}, err
	}

	userDomain := userDomainFromModel(userModel)

	span.SetAttributes(attribute.String("user.id", userModel.ID.String()))
	span.SetStatus(codes.Ok, "user found")

	return userDomain, nil
}
