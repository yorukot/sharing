package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Storage implements the Storage interface using S3-compatible storage
type S3Storage struct {
	client *s3.Client
	bucket string
}

// S3Config holds configuration for S3 storage
type S3Config struct {
	Endpoint        string
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
}

// NewS3Storage creates a new S3 storage backend
func NewS3Storage(config S3Config) (*S3Storage, error) {
	// Create custom resolver for endpoint
	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			if config.Endpoint != "" {
				return aws.Endpoint{
					URL:               config.Endpoint,
					HostnameImmutable: true,
					Source:            aws.EndpointSourceCustom,
				}, nil
			}
			// Return empty endpoint to use default
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		},
	)

	// Create AWS config
	cfg := aws.Config{
		Region: config.Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			config.AccessKeyID,
			config.SecretAccessKey,
			"",
		),
		EndpointResolverWithOptions: customResolver,
	}

	// Create S3 client
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = config.UsePathStyle
	})

	return &S3Storage{
		client: client,
		bucket: config.Bucket,
	}, nil
}

// Save uploads a file to S3
func (s *S3Storage) Save(reader io.Reader, filename string, size int64) (string, error) {
	ctx := context.Background()

	// Use filename as the S3 key
	key := filename

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   reader,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	return key, nil
}

// Get downloads a file from S3
func (s *S3Storage) Get(path string) (io.ReadCloser, error) {
	ctx := context.Background()

	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download from S3: %w", err)
	}

	return result.Body, nil
}

// Delete removes a file from S3
func (s *S3Storage) Delete(path string) error {
	ctx := context.Background()

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return fmt.Errorf("failed to delete from S3: %w", err)
	}

	return nil
}

// Exists checks if a file exists in S3
func (s *S3Storage) Exists(path string) (bool, error) {
	ctx := context.Background()

	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		// Check if error is "not found"
		return false, nil
	}

	return true, nil
}
