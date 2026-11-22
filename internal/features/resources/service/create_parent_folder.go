package resources_service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func (s *ResourcesService) CreateParentFolder(ctx context.Context, userID uuid.UUID) error {
	ctx, span := tracer.Start(ctx, "ResourcesService.CreateParentFolder")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", userID.String()))

	if err := s.resourcesRepository.CreateParentFolder(ctx, userID); err != nil {
		err = fmt.Errorf("resource_service - CreateParentFolder - resources.Repository.CreateParentFolder: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create parent folder in repository")
		return err
	}

	span.SetStatus(codes.Ok, "parent folder created successfully")

	return nil
}
