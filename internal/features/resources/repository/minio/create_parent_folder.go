package resources_repository_minio

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func (r *ResourcesRepository) CreateParentFolder(
	ctx context.Context,
	userID uuid.UUID,
) error {
	ctx, span := tracer.Start(ctx, "ResourcesRepository.CreateParentFolder")
	defer span.End()

	key := userRootFolderKey(userID)

	span.SetAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("storage.system", "minio"),
		attribute.String("storage.operation", "PutObject"),
		attribute.String("storage.bucket", r.mc.BucketName()),
	)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	span.AddEvent("creating user root folder in MinIO")

	if err := r.mc.CreateFolder(ctx, key); err != nil {
		err = fmt.Errorf("minio put parent folder %q: %w", key, err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create parent folder in MinIO")
		return err
	}

	span.SetStatus(codes.Ok, "parent folder created in MinIO")

	return nil
}
