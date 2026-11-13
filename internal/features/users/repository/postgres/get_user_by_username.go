package users_repository_postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	core_errors "github.com/vladpann/go-cloud-file-storage/internal/errors"
	user_service "github.com/vladpann/go-cloud-file-storage/internal/features/users/service"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func (r *UsersRepository) GetUserByUsername(
	ctx context.Context,
	username string,
) (user_service.UserModel, error) {
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
		return user_service.UserModel{}, err
	}

	span.SetAttributes(attribute.String("db.statement", sql))
	span.AddEvent("executing SQL query")

	row := r.pg.Pool.QueryRow(ctx, sql, args...)

	var userModel user_service.UserModel
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
			return user_service.UserModel{}, err
		}

		err = fmt.Errorf("users_repository_postgres - GetUserByUsername - row.Scan: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to scan row")
		return user_service.UserModel{}, err
	}

	span.SetAttributes(attribute.String("user.id", userModel.ID.String()))
	span.SetStatus(codes.Ok, "user found")

	return userModel, nil
}
