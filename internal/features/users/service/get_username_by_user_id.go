package user_service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func (s *UsersService) GetUsernameByUserID(
	ctx context.Context,
	userID uuid.UUID,
) (string, error) {
	ctx, span := tracer.Start(ctx, "UsersService.GetUserByUserID")
	defer span.End()

	span.SetAttributes(attribute.String("user.userID", userID.String()))

	// Get username by user id
	username, err := s.usersRepository.GetUsernameByUserID(ctx, userID)
	if err != nil {
		err = fmt.Errorf("user_service - GetUsernameByUserID - s.usersRepository.GetUsernameByUserID: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "username not found")
		return username, err
	}

	span.AddEvent("username found")
	span.SetAttributes(attribute.String("username", username))

	span.SetStatus(codes.Ok, "username in successful")

	return username, nil
}
