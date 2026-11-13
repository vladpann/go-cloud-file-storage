package user_service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func (s *UsersService) SignIn(
	ctx context.Context,
	username string,
	password string,
) (uuid.UUID, error) {
	ctx, span := tracer.Start(ctx, "UsersService.SignIn")
	defer span.End()

	span.SetAttributes(attribute.String("user.username", username))

	// Get user by username
	userModel, err := s.usersRepository.GetUserByUsername(ctx, username)
	if err != nil {
		err = fmt.Errorf("user_service - SignIn - s.usersRepository.GetUserByUsername: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "user not found")
		return uuid.Nil, err
	}

	span.AddEvent("user found")
	span.SetAttributes(attribute.String("user.id", userModel.ID.String()))

	// Compare password
	err = s.passwordHasher.Compare(userModel.PasswordHash, password)
	if err != nil {
		err = fmt.Errorf("user_service - SignIn - s.passwordHasher.Compare: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid password")
		return uuid.Nil, err
	}

	span.AddEvent("password verified")
	span.SetStatus(codes.Ok, "sign in successful")

	return userModel.ID, nil
}
