package resources_repository_minio

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	pkgminio "github.com/vladpann/go-cloud-file-storage/pkg/minio"
)

func (r *ResourcesRepository) GetFolderContent(
	ctx context.Context,
	userID uuid.UUID,
	dir string,
) ([]domain.Resource, error) {
	ctx, span := tracer.Start(ctx, "ResourcesRepository.GetFolderContent")
	defer span.End()

	if dir != "" && !strings.HasSuffix(dir, "/") {
		dir += "/"
	}

	key := userRootFolderKey(userID) + dir

	span.SetAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("storage.system", "minio"),
		attribute.String("storage.operation", "ListObjects"),
		attribute.String("storage.bucket", r.mc.BucketName()),
	)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	objects, err := r.mc.FolderContent(ctx, key)
	if err != nil {
		if errors.Is(err, pkgminio.ErrNotFound) {
			span.SetStatus(codes.Error, "folder not found")
			return nil, domain.ErrResourceNotFound
		}

		err = fmt.Errorf("minio list folder %q: %w", key, err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to list folder content in MinIO")
		return nil, err
	}

	resources := make([]ResourceModel, 0, len(objects))
	for i := range objects {
		obj := &objects[i]
		rel := strings.TrimPrefix(obj.Key, key)

		var res domain.Resource
		if before, ok := strings.CutSuffix(rel, "/"); ok {
			res, err = domain.NewDirectory(dir, before)
		} else {
			res, err = domain.NewFile(dir, rel, obj.Size)
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to build resource from MinIO object")
			return nil, fmt.Errorf("build resource for key %q: %w", obj.Key, err)
		}

		resources = append(resources, resourceModelFromDomain(res))
	}

	span.SetStatus(codes.Ok, "folder content listed from MinIO")

	folderContent := resourceDomainsFromModel(resources)

	return folderContent, nil
}
