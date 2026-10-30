// Package storage is the object store for images: clients upload straight to
// it with a presigned URL, so image bytes never pass through the service.
package storage

import (
	"context"
	"sync"
	"time"
)

// Upload is a presigned POST: the client sends Fields plus the file to URL.
type Upload struct {
	URL    string
	Fields map[string]string
}

type Storage interface {
	// PresignUpload allows one upload to key, of contentType and at most
	// maxBytes, for ttl. It also returns the public URL the object is served from.
	PresignUpload(ctx context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (Upload, string, error)
}

var (
	mu      sync.RWMutex
	current Storage
)

// Get returns the configured store, or nil when none is configured.
func Get() Storage {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Set installs the store; the platform calls it at startup, tests with a fake.
func Set(s Storage) {
	mu.Lock()
	defer mu.Unlock()
	current = s
}
