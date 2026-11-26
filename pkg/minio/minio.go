package minio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
	ErrAlreadyExists = errors.New("object already exists")
	ErrNotFound      = errors.New("object not found")
)

// Client wraps minio.Client with additional functionality.
type Client struct {
	*minio.Client
	bucketName string
}

// Config holds MinIO client configuration.
type Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	BucketName      string
}

// New creates a new MinIO client and ensures the bucket exists.
func New(ctx context.Context, cfg Config) (*Client, error) {
	// Initialize MinIO client
	minioClient, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	client := &Client{
		Client:     minioClient,
		bucketName: cfg.BucketName,
	}

	// Ensure bucket exists
	if err := client.ensureBucketExists(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure bucket exists: %w", err)
	}

	return client, nil
}

// ensureBucketExists checks if the bucket exists and creates it if it doesn't.
func (c *Client) ensureBucketExists(ctx context.Context) error {
	exists, err := c.BucketExists(ctx, c.bucketName)
	if err != nil {
		return fmt.Errorf("failed to check if bucket exists: %w", err)
	}

	if !exists {
		err = c.MakeBucket(ctx, c.bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return nil
}

// BucketName returns the configured bucket name.
func (c *Client) BucketName() string {
	return c.bucketName
}

// PutObject uploads an object into the configured bucket.
func (c *Client) PutObject(
	ctx context.Context,
	objectName string,
	reader io.Reader,
	objectSize int64,
	opts *minio.PutObjectOptions,
) (minio.UploadInfo, error) {
	var putOpts minio.PutObjectOptions
	if opts != nil {
		putOpts = *opts
	}

	return c.Client.PutObject(ctx, c.bucketName, objectName, reader, objectSize, putOpts)
}

// CreateFolder creates a zero-byte directory marker object (key must end with "/").
func (c *Client) CreateFolder(ctx context.Context, folderKey string) error {
	if !strings.HasSuffix(folderKey, "/") {
		folderKey += "/"
	}

	exists, err := c.FolderExists(ctx, folderKey)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: %s", ErrAlreadyExists, folderKey)
	}

	_, err = c.Client.PutObject(
		ctx,
		c.bucketName,
		folderKey,
		bytes.NewReader(nil),
		0,
		minio.PutObjectOptions{
			ContentType: "application/x-directory",
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create folder %q: %w", folderKey, err)
	}

	return nil
}

// FolderExists checks whether a folder marker or any object under folderKey exists.
func (c *Client) FolderExists(ctx context.Context, folderKey string) (bool, error) {
	if !strings.HasSuffix(folderKey, "/") {
		folderKey += "/"
	}

	opts := minio.ListObjectsOptions{
		Prefix:    folderKey,
		Recursive: true,
		MaxKeys:   1,
	}

	obj, ok := <-c.ListObjects(ctx, c.bucketName, opts)
	if !ok {
		return false, nil
	}
	if obj.Err != nil {
		return false, fmt.Errorf(
			"failed to check folder %q: %w",
			folderKey,
			obj.Err,
		)
	}

	return true, nil
}

// FolderContent returns direct children (files and subfolders) under folderKey.
// Returns ErrNotFound if neither the folder marker nor any objects with this prefix exist.
func (c *Client) FolderContent(ctx context.Context, folderKey string) ([]minio.ObjectInfo, error) {
	if !strings.HasSuffix(folderKey, "/") {
		folderKey += "/"
	}

	opts := minio.ListObjectsOptions{
		Prefix:    folderKey,
		Recursive: false,
	}

	objects := make([]minio.ObjectInfo, 0)
	seen := make(map[string]struct{})
	found := false

	for obj := range c.ListObjects(ctx, c.bucketName, opts) {
		if obj.Err != nil {
			return nil, fmt.Errorf(
				"failed to list folder content %q: %w",
				folderKey,
				obj.Err,
			)
		}

		found = true

		if obj.Key == folderKey {
			continue
		}

		if _, exists := seen[obj.Key]; exists {
			continue
		}
		seen[obj.Key] = struct{}{}

		objects = append(objects, obj)
	}

	if !found {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, folderKey)
	}

	return objects, nil
}

// ObjectExists checks whether an object with exact objectKey exists in the bucket.
func (c *Client) ObjectExists(ctx context.Context, objectKey string) (bool, error) {
	_, err := c.StatObject(ctx, c.bucketName, objectKey, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat object %q: %w", objectKey, err)
	}

	return true, nil
}

// EnsureFolderExists creates a zero-byte folder marker if it does not already exist.
func (c *Client) EnsureFolderExists(ctx context.Context, folderKey string) error {
	if !strings.HasSuffix(folderKey, "/") {
		folderKey += "/"
	}

	exists, err := c.FolderExists(ctx, folderKey)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	_, err = c.Client.PutObject(
		ctx,
		c.bucketName,
		folderKey,
		bytes.NewReader(nil),
		0,
		minio.PutObjectOptions{
			ContentType: "application/x-directory",
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create folder %q: %w", folderKey, err)
	}

	return nil
}
