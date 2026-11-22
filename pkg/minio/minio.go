package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
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

	_, err := c.Client.PutObject(
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
