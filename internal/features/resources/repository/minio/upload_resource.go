package resources_repository_minio

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
)

type preparedUpload struct {
	upload    *domain.UploadFile
	parentDir string
	fileName  string
	objectKey string
}

func (r *ResourcesRepository) UploadResource(
	ctx context.Context,
	userID uuid.UUID,
	targetDir string,
	uploads []domain.UploadFile,
) ([]domain.Resource, error) {
	ctx, span := tracer.Start(ctx, "ResourcesRepository.UploadResource")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("storage.system", "minio"),
		attribute.String("storage.operation", "PutObject"),
		attribute.String("storage.bucket", r.mc.BucketName()),
		attribute.Int("upload.files_count", len(uploads)),
	)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	rootKey := userRootFolderKey(userID)

	if targetDir != "" {
		if err := r.ensureTargetFolderExists(ctx, rootKey+targetDir); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "target folder check failed")
			return nil, err
		}
	}

	prepared, dirsToEnsure, err := r.prepareUploads(ctx, rootKey, targetDir, uploads)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "upload preparation failed")
		return nil, err
	}

	if err := r.ensureFolders(ctx, dirsToEnsure); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create intermediate folders")
		return nil, err
	}

	resources, err := r.uploadPreparedObjects(ctx, prepared)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to upload objects to MinIO")
		return nil, err
	}

	span.SetStatus(codes.Ok, "resources uploaded to MinIO")

	return resources, nil
}

func (r *ResourcesRepository) ensureTargetFolderExists(ctx context.Context, folderKey string) error {
	exists, err := r.mc.FolderExists(ctx, folderKey)
	if err != nil {
		return fmt.Errorf("minio check target folder %q: %w", folderKey, err)
	}
	if !exists {
		return domain.ErrResourceNotFound
	}
	return nil
}

func (r *ResourcesRepository) prepareUploads(
	ctx context.Context,
	rootKey string,
	targetDir string,
	uploads []domain.UploadFile,
) ([]preparedUpload, map[string]struct{}, error) {
	prepared := make([]preparedUpload, 0, len(uploads))
	dirsToEnsure := make(map[string]struct{})

	for i := range uploads {
		upload := &uploads[i]

		relParent, fileName, _, err := domain.ParseResourcePath(upload.RelativePath)
		if err != nil {
			return nil, nil, err
		}

		parentDir := targetDir + relParent
		objectKey := rootKey + parentDir + fileName

		exists, err := r.mc.ObjectExists(ctx, objectKey)
		if err != nil {
			return nil, nil, fmt.Errorf("minio check object %q: %w", objectKey, err)
		}
		if exists {
			return nil, nil, domain.ErrResourceAlreadyExists
		}

		if relParent != "" {
			for _, dirKey := range collectIntermediateFolderKeys(rootKey+targetDir, relParent) {
				dirsToEnsure[dirKey] = struct{}{}
			}
		}

		prepared = append(prepared, preparedUpload{
			upload:    upload,
			parentDir: parentDir,
			fileName:  fileName,
			objectKey: objectKey,
		})
	}

	return prepared, dirsToEnsure, nil
}

func (r *ResourcesRepository) ensureFolders(ctx context.Context, dirs map[string]struct{}) error {
	for folderKey := range dirs {
		if err := r.mc.EnsureFolderExists(ctx, folderKey); err != nil {
			return fmt.Errorf("minio ensure folder %q: %w", folderKey, err)
		}
	}
	return nil
}

func (r *ResourcesRepository) uploadPreparedObjects(ctx context.Context, prepared []preparedUpload) ([]domain.Resource, error) {
	resources := make([]domain.Resource, 0, len(prepared))
	uploadedKeys := make([]string, 0, len(prepared))

	for i := range prepared {
		item := &prepared[i]

		_, err := r.mc.PutObject(
			ctx,
			item.objectKey,
			item.upload.Reader,
			item.upload.Size,
			&minio.PutObjectOptions{
				ContentType: item.upload.ContentType,
			},
		)
		if err != nil {
			if rbErr := r.rollbackUploadedObjects(ctx, uploadedKeys); rbErr != nil {
				err = errors.Join(err, rbErr)
			}
			return nil, fmt.Errorf("minio put object %q: %w", item.objectKey, err)
		}

		uploadedKeys = append(uploadedKeys, item.objectKey)

		res, err := domain.NewFile(item.parentDir, item.fileName, item.upload.Size)
		if err != nil {
			return nil, err
		}

		resources = append(resources, res)
	}

	return resources, nil
}

func collectIntermediateFolderKeys(baseKey, relParent string) []string {
	trimmed := strings.TrimSuffix(relParent, "/")
	if trimmed == "" {
		return nil
	}

	parts := strings.Split(trimmed, "/")
	keys := make([]string, 0, len(parts))
	cur := baseKey
	for _, part := range parts {
		cur += part + "/"
		keys = append(keys, cur)
	}
	return keys
}

func (r *ResourcesRepository) rollbackUploadedObjects(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	var errs []error
	for _, key := range keys {
		if err := r.mc.RemoveObject(cleanupCtx, r.mc.BucketName(), key, minio.RemoveObjectOptions{}); err != nil {
			errs = append(errs, fmt.Errorf("rollback remove object %q: %w", key, err))
		}
	}

	return errors.Join(errs...)
}
