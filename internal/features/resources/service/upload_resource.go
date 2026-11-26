package resources_service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
)

func (s *ResourcesService) UploadResource(
	ctx context.Context,
	userID uuid.UUID,
	rawPath string,
	uploads []domain.UploadFile,
) ([]domain.Resource, error) {
	ctx, span := tracer.Start(ctx, "ResourcesService.UploadResource")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.id", userID.String()),
		attribute.Int("upload.files_count", len(uploads)),
	)

	targetDir, err := domain.ParseTargetDirectory(rawPath)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid target directory path")
		return nil, err
	}

	seen := make(map[string]struct{}, len(uploads))
	for i := range uploads {
		upload := &uploads[i]

		if upload.Reader == nil {
			err := fmt.Errorf("resources_service - UploadResource: nil file reader: %w", domain.ErrInvalidPath)
			span.RecordError(err)
			span.SetStatus(codes.Error, "nil file reader")
			return nil, err
		}
		if upload.Size < 0 {
			span.RecordError(domain.ErrInvalidSize)
			span.SetStatus(codes.Error, "negative file size")
			return nil, domain.ErrInvalidSize
		}

		relParent, fileName, resType, err := domain.ParseResourcePath(upload.RelativePath)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "invalid uploaded file path")
			return nil, err
		}
		if resType != domain.ResourceTypeFile {
			err = fmt.Errorf("resources_service - UploadResource: uploaded item must be file: %w", domain.ErrInvalidPath)
			span.RecordError(err)
			span.SetStatus(codes.Error, "uploaded item is not a file")
			return nil, err
		}

		fullParent := targetDir + relParent
		if _, err := domain.NewFile(fullParent, fileName, upload.Size); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "invalid file resource")
			return nil, err
		}

		fullKey := fullParent + fileName
		if _, exists := seen[fullKey]; exists {
			err = fmt.Errorf("resources_service - UploadResource: duplicate file %q in request: %w", fullKey, domain.ErrInvalidPath)
			span.RecordError(err)
			span.SetStatus(codes.Error, "duplicate file in request")
			return nil, err
		}
		seen[fullKey] = struct{}{}
	}

	resources, err := s.resourcesRepository.UploadResource(ctx, userID, targetDir, uploads)
	if err != nil {
		err = fmt.Errorf("resources_service - UploadResource - resourcesRepository.UploadResource: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to upload resource")
		return nil, err
	}

	span.SetStatus(codes.Ok, "resources uploaded")

	return resources, nil
}
