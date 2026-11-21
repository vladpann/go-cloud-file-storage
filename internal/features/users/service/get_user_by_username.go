package user_service

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
)

func (s *UsersService) GetUserByUsername(
	ctx context.Context,
	username string,
) (domain.User, error) {
	ctx, span := tracer.Start(ctx, "UsersService.GetUserByUsername")
	defer span.End()

	span.SetAttributes(attribute.String("user.username", username))

	// Get user by username
	user, err := s.usersRepository.GetUserByUsername(ctx, username)
	if err != nil {
		err = fmt.Errorf("user_service - GetUserByUsername - s.usersRepository.GetUserByUsername: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "user not found")
		return domain.User{}, err
	}

	span.AddEvent("user found")
	span.SetAttributes(attribute.String("user.id", user.ID.String()))

	span.SetStatus(codes.Ok, "user in successful")

	return user, nil
}
