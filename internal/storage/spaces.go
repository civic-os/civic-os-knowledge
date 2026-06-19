package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ObjectStore abstracts S3-compatible object storage for testability.
type ObjectStore interface {
	ListObjects(ctx context.Context, prefix string) ([]string, error)
	GetObject(ctx context.Context, key string) ([]byte, error)
	PutObject(ctx context.Context, key string, data []byte) error
}

// SpacesStore implements ObjectStore for DigitalOcean Spaces (S3-compatible).
type SpacesStore struct {
	client *s3.Client
	bucket string
	prefix string
}

// SpacesConfig holds configuration for connecting to DO Spaces.
type SpacesConfig struct {
	Endpoint  string // e.g. "https://nyc3.digitaloceanspaces.com"
	Region    string // e.g. "nyc3"
	Bucket    string
	Prefix    string // e.g. "knowledge/"
	AccessKey string
	SecretKey string
	PathStyle bool   // true for MinIO, false for DO Spaces
}

// NewSpacesStore creates a SpacesStore from config.
func NewSpacesStore(ctx context.Context, cfg SpacesConfig) (*SpacesStore, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.PathStyle
	})

	return &SpacesStore{
		client: client,
		bucket: cfg.Bucket,
		prefix: cfg.Prefix,
	}, nil
}

func (s *SpacesStore) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	fullPrefix := s.prefix + prefix
	input := &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: &fullPrefix,
	}

	var keys []string
	paginator := s3.NewListObjectsV2Paginator(s.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}
		for _, obj := range page.Contents {
			key := strings.TrimPrefix(*obj.Key, s.prefix)
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (s *SpacesStore) GetObject(ctx context.Context, key string) ([]byte, error) {
	fullKey := s.prefix + key
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &fullKey,
	})
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", key, err)
	}
	defer output.Body.Close()
	return io.ReadAll(output.Body)
}

func (s *SpacesStore) PutObject(ctx context.Context, key string, data []byte) error {
	fullKey := s.prefix + key
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &s.bucket,
		Key:    &fullKey,
		Body:   strings.NewReader(string(data)),
	})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

// Syncer handles pulling from and pushing to S3.
type Syncer struct {
	store   ObjectStore
	localDir string
	pushCh  chan pushRequest
}

type pushRequest struct {
	key  string
	data []byte
}

// NewSyncer creates a Syncer that pushes asynchronously.
func NewSyncer(store ObjectStore, localDir string) *Syncer {
	s := &Syncer{
		store:    store,
		localDir: localDir,
		pushCh:   make(chan pushRequest, 100),
	}
	go s.pushLoop()
	return s
}

// Pull downloads all objects from S3 to the local directory.
func (s *Syncer) Pull(ctx context.Context) error {
	keys, err := s.store.ListObjects(ctx, "")
	if err != nil {
		return fmt.Errorf("list for pull: %w", err)
	}

	for _, key := range keys {
		data, err := s.store.GetObject(ctx, key)
		if err != nil {
			log.Printf("WARNING: pull %s: %v", key, err)
			continue
		}

		localPath := filepath.Join(s.localDir, key)
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", key, err)
		}
		if err := os.WriteFile(localPath, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", key, err)
		}
	}

	log.Printf("pulled %d objects from S3", len(keys))
	return nil
}

// PushFile queues a file for async upload to S3.
func (s *Syncer) PushFile(path string) {
	localPath := filepath.Join(s.localDir, path)
	data, err := os.ReadFile(localPath)
	if err != nil {
		log.Printf("WARNING: read for push %s: %v", path, err)
		return
	}
	s.pushCh <- pushRequest{key: path, data: data}
}

// Close stops the push loop.
func (s *Syncer) Close() {
	close(s.pushCh)
}

func (s *Syncer) pushLoop() {
	for req := range s.pushCh {
		if err := s.store.PutObject(context.Background(), req.key, req.data); err != nil {
			log.Printf("WARNING: S3 push %s: %v", req.key, err)
		}
	}
}
