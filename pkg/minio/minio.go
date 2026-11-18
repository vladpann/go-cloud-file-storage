package minio

import (
	"context"
	"fmt"

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
