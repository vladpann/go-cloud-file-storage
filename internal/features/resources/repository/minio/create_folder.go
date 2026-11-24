package resources_repository_minio

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	pkgminio "github.com/vladpann/go-cloud-file-storage/pkg/minio"
)

func (r *ResourcesRepository) CreateFolder(
	ctx context.Context,
	userID uuid.UUID,
	dir domain.Resource,
) error {
	ctx, span := tracer.Start(ctx, "ResourcesRepository.CreateFolder")
	defer span.End()

	key := userRootFolderKey(userID) + dir.FullPath()

	span.SetAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("storage.system", "minio"),
		attribute.String("storage.operation", "PutObject"),
		attribute.String("storage.bucket", r.mc.BucketName()),
	)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := r.mc.CreateFolder(ctx, key); err != nil {
		if errors.Is(err, pkgminio.ErrAlreadyExists) {
			span.RecordError(err)
			span.SetStatus(codes.Error, "folder already exists")
			return domain.ErrResourceAlreadyExists
		}

		err = fmt.Errorf("minio create folder %q: %w", key, err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create folder in MinIO")
		return err
	}

	span.SetStatus(codes.Ok, "folder created in MinIO")

	return nil
}
