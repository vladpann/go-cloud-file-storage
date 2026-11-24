package resources_service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
)

func (s *ResourcesService) CreateEmptyFolder(ctx context.Context, userID uuid.UUID, rawPath string) (domain.Resource, error) {
	ctx, span := tracer.Start(ctx, "ResourcesService.CreateEmptyFolder")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", userID.String()))

	// Parses a full path
	parentPath, name, resType, err := domain.ParseResourcePath(rawPath)
	if err != nil {
		return domain.Resource{}, err
	}

	if resType != domain.ResourceTypeDirectory {
		err = fmt.Errorf("resources_service - domain.ResourceTypeDirectory: %w", domain.ErrInvalidPath)
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid or missing path")
		return domain.Resource{}, domain.ErrInvalidPath
	}

	dir, err := domain.NewDirectory(parentPath, name)
	if err != nil {
		err = fmt.Errorf("resources_service - CreateEmptyFolder - domain.NewDirectory: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create domain new directory")
		return domain.Resource{}, err
	}

	if err := s.resourcesRepository.CreateFolder(ctx, userID, dir); err != nil {
		err = fmt.Errorf("resources_service - CreateEmptyFolder - resourcesRepository.CreateFolder: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create folder")
		return domain.Resource{}, err
	}

	return dir, nil
}
