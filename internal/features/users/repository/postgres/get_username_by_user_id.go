package users_repository_postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	core_errors "github.com/vladpann/go-cloud-file-storage/internal/core/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func (r *UsersRepository) GetUsernameByUserID(
	ctx context.Context,
	userID uuid.UUID,
) (string, error) {
	ctx, span := tracer.Start(ctx, "UsersRepository.GetUsernameByUserID")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.userID", userID.String()),
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.table", "cloud_file_storage.users"),
	)

	sql, args, err := r.pg.Builder.
		Select("username").
		From("cloud_file_storage.users").
		Where("id = ?", userID).
		ToSql()
	if err != nil {
		err = fmt.Errorf("users_repository_postgres - GetUsernameByUserID - r.pg.Builder: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to build SQL query")
		return "", err
	}

	span.SetAttributes(attribute.String("db.statement", sql))
	span.AddEvent("executing SQL query")

	row := r.pg.Pool.QueryRow(ctx, sql, args...)

	var username string
	err = row.Scan(
		&username,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = fmt.Errorf("users_repository_postgres - GetUsernameByUserID: %w", core_errors.ErrNotFound)
			span.RecordError(err)
			span.SetStatus(codes.Error, "username not found")
			return "", err
		}

		err = fmt.Errorf("users_repository_postgres - GetUsernameByUserID - row.Scan: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to scan row")
		return "", err
	}

	span.SetAttributes(attribute.String("username", username))
	span.SetStatus(codes.Ok, "username found")

	return username, nil
}
