package user_service

import (
	"context"
	"fmt"

	"github.com/vladpann/go-cloud-file-storage/internal/domain"
	core_errors "github.com/vladpann/go-cloud-file-storage/internal/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("github.com/vladpann/go-cloud-file-storage/internal/features/users/service")

func (s *UsersService) SignUp(
	ctx context.Context,
	username string,
	password string,
	repeatPassword string,
) (domain.User, error) {
	ctx, span := tracer.Start(ctx, "UsersService.SignUp")
	defer span.End()

	span.SetAttributes(attribute.String("user.username", username))

	// Validate passwords match
	if password != repeatPassword {
		err := fmt.Errorf("user_service - SignUp: %w", core_errors.ErrInvalidArgument)
		span.RecordError(err)
		span.SetStatus(codes.Error, "passwords do not match")
		return domain.User{}, err
	}

	span.AddEvent("passwords validated")

	// Hash password
	passwordHash, err := s.passwordHasher.Hash(password)
	if err != nil {
		err = fmt.Errorf("user_service - SignUp - s.passwordHasher.Hash: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to hash password")
		return domain.User{}, err
	}

	span.AddEvent("password hashed")

	// Create user
	user, err := s.usersRepository.SignUp(ctx, username, passwordHash)
	if err != nil {
		err = fmt.Errorf("user_service - SignUp - s.usersRepository.SignUp: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create user in repository")
		return domain.User{}, err
	}

	span.SetAttributes(attribute.String("user.id", user.ID.String()))
	span.SetStatus(codes.Ok, "user created successfully")

	return user, nil
}
