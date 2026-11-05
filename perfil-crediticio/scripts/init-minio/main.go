package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	bucketName      = "credit-profile-assets"
	defaultEndpoint = "http://minio:9000"
	seedDataPath    = "../../data/seed"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx := context.Background()

	// Get configuration from environment variables
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	accessKey := os.Getenv("MINIO_ACCESS_KEY")
	if accessKey == "" {
		slog.Error("MINIO_ACCESS_KEY environment variable is required")
		os.Exit(1)
	}

	secretKey := os.Getenv("MINIO_SECRET_KEY")
	if secretKey == "" {
		slog.Error("MINIO_SECRET_KEY environment variable is required")
		os.Exit(1)
	}

	slog.Info("Initializing MinIO client", "endpoint", endpoint)

	// Create S3 client configured for MinIO
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion("us-east-1"), // MinIO doesn't care about region, but SDK requires it
	)
	if err != nil {
		slog.Error("Failed to load AWS config", "error", err)
		os.Exit(1)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	// Create bucket if it doesn't exist
	if err := createBucket(ctx, client, bucketName); err != nil {
		slog.Error("Failed to create bucket", "bucket", bucketName, "error", err)
		os.Exit(1)
	}

	// Upload seed data
	if err := uploadSeedData(ctx, client, bucketName, seedDataPath); err != nil {
		slog.Error("Failed to upload seed data", "error", err)
		os.Exit(1)
	}

	slog.Info("MinIO initialization completed successfully")
}

// createBucket creates the bucket if it doesn't exist
func createBucket(ctx context.Context, client *s3.Client, bucket string) error {
	slog.Info("Checking if bucket exists", "bucket", bucket)

	// Check if bucket exists
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})

	if err == nil {
		slog.Info("Bucket already exists", "bucket", bucket)
		return nil
	}

	// Bucket doesn't exist, create it
	slog.Info("Creating bucket", "bucket", bucket)
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
		ACL:    types.BucketCannedACLPrivate,
	})

	if err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	slog.Info("Bucket created successfully", "bucket", bucket)
	return nil
}

// uploadSeedData recursively uploads all files from the seed data directory
func uploadSeedData(ctx context.Context, client *s3.Client, bucket, rootPath string) error {
	slog.Info("Starting seed data upload", "path", rootPath)

	// Get absolute path to seed data
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if seed data directory exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("seed data directory does not exist: %s", absPath)
	}

	fileCount := 0

	// Walk through all files in the seed data directory
	err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Calculate the S3 key (relative path from seed data root)
		relPath, err := filepath.Rel(absPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Convert Windows path separators to forward slashes for S3
		s3Key := filepath.ToSlash(relPath)

		// Upload the file
		if err := uploadFile(ctx, client, bucket, s3Key, path); err != nil {
			return fmt.Errorf("failed to upload file %s: %w", relPath, err)
		}

		fileCount++
		return nil
	})

	if err != nil {
		return err
	}

	slog.Info("Seed data upload completed", "files_uploaded", fileCount)
	return nil
}

// uploadFile uploads a single file to S3
func uploadFile(ctx context.Context, client *s3.Client, bucket, key, filePath string) error {
	slog.Info("Uploading file", "key", key, "path", filePath)

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info for size
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Determine content type based on file extension
	contentType := getContentType(filePath)

	// Upload the file
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          file,
		ContentLength: aws.Int64(fileInfo.Size()),
		ContentType:   aws.String(contentType),
	})

	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}

	slog.Info("File uploaded successfully", "key", key, "size_bytes", fileInfo.Size())
	return nil
}

// getContentType returns the content type based on file extension
func getContentType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".html":
		return "text/html"
	case ".xml":
		return "application/xml"
	default:
		return "application/octet-stream"
	}
}
