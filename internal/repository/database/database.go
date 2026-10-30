// Package database is the in-memory stand-in for PostgreSQL in SIS #2: the
// benchmark compares protocols, so storage must cost the same for every one of
// them. SIS #3 replaces it with PostgreSQL behind the same repository API.
package database

import (
	"context"
	"sync"
)

var (
	mu       sync.RWMutex
	resetsMu sync.Mutex
	resets   []func()
)

type txKey struct{}

// Transactional runs fn with the store's write lock held; the returned context
// marks the transaction, so repositories called with it skip their own locks.
// There is no rollback: callers validate first and write last.
func Transactional(ctx context.Context, fn func(ctx context.Context) error) error {
	if inTx(ctx) {
		return fn(ctx)
	}
	mu.Lock()
	defer mu.Unlock()
	return fn(context.WithValue(ctx, txKey{}, true))
}

func inTx(ctx context.Context) bool {
	v, _ := ctx.Value(txKey{}).(bool)
	return v
}

// ReadLock and WriteLock are no-ops inside Transactional.
func ReadLock(ctx context.Context) func() {
	if inTx(ctx) {
		return func() {}
	}
	mu.RLock()
	return mu.RUnlock
}

func WriteLock(ctx context.Context) func() {
	if inTx(ctx) {
		return func() {}
	}
	mu.Lock()
	return mu.Unlock
}

// RegisterTable records how to empty a table for ReloadInMemoryDB.
func RegisterTable(reset func()) {
	resetsMu.Lock()
	defer resetsMu.Unlock()
	resets = append(resets, reset)
}

// ReloadInMemoryDB empties every table; tests call it in their setup.
func ReloadInMemoryDB() {
	mu.Lock()
	defer mu.Unlock()
	resetsMu.Lock()
	defer resetsMu.Unlock()
	for _, reset := range resets {
		reset()
	}
}
