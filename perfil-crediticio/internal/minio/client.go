package minio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const cacheTTL = 30 * time.Second

// Client es un cliente MinIO/S3 con cache en memoria
type Client struct {
	client *s3.Client
	bucket string
	cache  sync.Map // key: string, value: cacheEntry
}

type cacheEntry struct {
	data      []byte
	timestamp time.Time
}

// NewClient crea un nuevo cliente MinIO/S3
func NewClient(endpoint, accessKey, secretKey, bucket string) (*Client, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("minio NewClient: failed to load config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	slog.Info("minio client initialized",
		"endpoint", endpoint,
		"bucket", bucket,
	)

	return &Client{
		client: client,
		bucket: bucket,
		cache:  sync.Map{},
	}, nil
}

// GetObjectJSON descarga un objeto JSON y lo deserializa
// Utiliza cache en memoria con TTL de 30s
func (c *Client) GetObjectJSON(ctx context.Context, prefix, key string, target any) error {
	fullKey := prefix + "/" + key
	if prefix == "" {
		fullKey = key
	}

	// Verificar cache
	if entry, ok := c.cache.Load(fullKey); ok {
		cached := entry.(cacheEntry)
		if time.Since(cached.timestamp) < cacheTTL {
			slog.DebugContext(ctx, "cache hit", "key", fullKey)
			return json.Unmarshal(cached.data, target)
		}
		// Cache expirado, eliminar
		c.cache.Delete(fullKey)
	}

	// Descargar de MinIO
	result, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return fmt.Errorf("minio GetObjectJSON: failed to get object %s: %w", fullKey, err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return fmt.Errorf("minio GetObjectJSON: failed to read object body: %w", err)
	}

	// Guardar en cache
	c.cache.Store(fullKey, cacheEntry{
		data:      data,
		timestamp: time.Now(),
	})

	// Deserializar
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("minio GetObjectJSON: failed to unmarshal JSON: %w", err)
	}

	slog.DebugContext(ctx, "object downloaded and cached", "key", fullKey)
	return nil
}

// GetObjectText descarga un objeto como texto plano
// Utiliza el mismo sistema de cache
func (c *Client) GetObjectText(ctx context.Context, prefix, key string) (string, error) {
	fullKey := prefix + "/" + key
	if prefix == "" {
		fullKey = key
	}

	// Verificar cache
	if entry, ok := c.cache.Load(fullKey); ok {
		cached := entry.(cacheEntry)
		if time.Since(cached.timestamp) < cacheTTL {
			slog.DebugContext(ctx, "cache hit", "key", fullKey)
			return string(cached.data), nil
		}
		c.cache.Delete(fullKey)
	}

	// Descargar de MinIO
	result, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return "", fmt.Errorf("minio GetObjectText: failed to get object %s: %w", fullKey, err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return "", fmt.Errorf("minio GetObjectText: failed to read object body: %w", err)
	}

	// Guardar en cache
	c.cache.Store(fullKey, cacheEntry{
		data:      data,
		timestamp: time.Now(),
	})

	return string(data), nil
}

// PutObject sube un objeto a MinIO
func (c *Client) PutObject(ctx context.Context, key string, data []byte) error {
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          aws.ReadSeekCloser(io.NopCloser(bytes.NewReader(data))),
		ContentLength: aws.Int64(int64(len(data))),
	})

	if err != nil {
		return fmt.Errorf("minio PutObject: failed to put object %s: %w", key, err)
	}

	// Invalidar cache si existe
	c.cache.Delete(key)

	slog.InfoContext(ctx, "object uploaded", "key", key, "size", len(data))
	return nil
}

// ClearCache limpia el cache completamente
func (c *Client) ClearCache() {
	c.cache.Range(func(key, value interface{}) bool {
		c.cache.Delete(key)
		return true
	})
	slog.Info("minio cache cleared")
}
