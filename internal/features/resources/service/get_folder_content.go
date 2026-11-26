package resources_service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
)

func (s *ResourcesService) GetFolderContent(
	ctx context.Context,
	userID uuid.UUID,
	rawPath string,
) ([]domain.Resource, error) {
	ctx, span := tracer.Start(ctx, "ResourcesService.GetInfoContent")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", userID.String()))

	var dir string
	if rawPath != "" && rawPath != "/" {
		parentPath, name, resType, err := domain.ParseResourcePath(rawPath)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "invalid or missing path")
			return nil, err
		}

		if resType != domain.ResourceTypeDirectory {
			err = fmt.Errorf(
				"resources_service - domain.ResourceTypeDirectory: %w",
				domain.ErrInvalidPath,
			)
			span.RecordError(err)
			span.SetStatus(codes.Error, "invalid or missing path")
			return nil, domain.ErrInvalidPath
		}

		dir = parentPath + name + "/"
	}

	resources, err := s.resourcesRepository.GetFolderContent(ctx, userID, dir)
	if err != nil {
		err = fmt.Errorf(
			"resources_service - GetFolderContent - resourcesRepository.GetFolderContent: %w",
			err,
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get info folder content")
		return []domain.Resource{}, err
	}

	return resources, nil
}
