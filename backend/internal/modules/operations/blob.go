package operations

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
)

type BlobStore interface {
	Put(context.Context, string, string, io.Reader) error
	Open(context.Context, string) (io.ReadCloser, error)
}

type MemoryBlobStore struct {
	mu    sync.Mutex
	blobs map[string][]byte
}

func NewMemoryBlobStore() *MemoryBlobStore { return &MemoryBlobStore{blobs: make(map[string][]byte)} }

func (s *MemoryBlobStore) Put(_ context.Context, name, _ string, source io.Reader) error {
	value, err := io.ReadAll(source)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blobs[name] = append([]byte(nil), value...)
	return nil
}

func (s *MemoryBlobStore) Open(_ context.Context, name string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.blobs[name]
	if !ok {
		return nil, errNotFound
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), value...))), nil
}

type GCSBlobStore struct{ bucket *storage.BucketHandle }

func NewGCSBlobStore(client *storage.Client, bucket string) *GCSBlobStore {
	return &GCSBlobStore{bucket: client.Bucket(bucket)}
}

func (s *GCSBlobStore) Put(ctx context.Context, name, contentType string, source io.Reader) error {
	writer := s.bucket.Object(name).NewWriter(ctx)
	writer.ContentType = contentType
	writer.CacheControl = "private, max-age=0, no-store"
	if _, err := io.Copy(writer, source); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

func (s *GCSBlobStore) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	reader, err := s.bucket.Object(name).NewReader(ctx)
	var apiError *googleapi.Error
	if errors.As(err, &apiError) && apiError.Code == 404 {
		return nil, errNotFound
	}
	return reader, err
}
