// Copyright 2018 Google LLC All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package registry

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// dedupBlob represents a blob with reference counting
type dedupBlob struct {
	data       []byte
	refCount   int64
	lastAccess time.Time
	repos      map[string]bool // Track which repos reference this blob
	mu         sync.RWMutex
}

// dedupHandler implements BlobHandler with deduplication and reference counting
type dedupHandler struct {
	blobs map[string]*dedupBlob
	mu    sync.RWMutex

	// Stats for monitoring
	totalSize   int64
	uniqueBlobs int64
	totalRefs   int64
}

// NewDedupBlobHandler creates a new deduplicated blob handler
func NewDedupBlobHandler() BlobHandler {
	return &dedupHandler{
		blobs: make(map[string]*dedupBlob),
	}
}

// Stat returns the size of the blob if it exists
func (d *dedupHandler) Stat(ctx context.Context, repo string, h v1.Hash) (int64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	blob, exists := d.blobs[h.String()]
	if !exists {
		return 0, errNotFound
	}

	blob.mu.RLock()
	defer blob.mu.RUnlock()

	// Check if this repo has access to this blob
	if !blob.repos[repo] {
		return 0, errNotFound
	}

	return int64(len(blob.data)), nil
}

// Get retrieves a blob by hash
func (d *dedupHandler) Get(ctx context.Context, repo string, h v1.Hash) (io.ReadCloser, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	blob, exists := d.blobs[h.String()]
	if !exists {
		return nil, errNotFound
	}

	blob.mu.Lock()
	defer blob.mu.Unlock()

	// Check if this repo has access to this blob
	if !blob.repos[repo] {
		return nil, errNotFound
	}

	// Update last access time
	blob.lastAccess = time.Now()

	return &bytesCloser{bytes.NewReader(blob.data)}, nil
}

// Put stores a blob with deduplication
func (d *dedupHandler) Put(ctx context.Context, repo string, h v1.Hash, rc io.ReadCloser) error {
	defer rc.Close()

	// Read the blob data
	data, err := io.ReadAll(rc)
	if err != nil {
		return err
	}

	digest := h.String()

	d.mu.Lock()
	defer d.mu.Unlock()

	if blob, exists := d.blobs[digest]; exists {
		// Blob already exists, increment reference count
		blob.mu.Lock()
		defer blob.mu.Unlock()

		if !blob.repos[repo] {
			blob.repos[repo] = true
			atomic.AddInt64(&blob.refCount, 1)
			atomic.AddInt64(&d.totalRefs, 1)
		}
		blob.lastAccess = time.Now()
		return nil
	}

	// New blob, create entry
	newBlob := &dedupBlob{
		data:       data,
		refCount:   1,
		lastAccess: time.Now(),
		repos:      map[string]bool{repo: true},
	}

	d.blobs[digest] = newBlob
	atomic.AddInt64(&d.totalSize, int64(len(data)))
	atomic.AddInt64(&d.uniqueBlobs, 1)
	atomic.AddInt64(&d.totalRefs, 1)

	return nil
}

// Delete removes a blob reference from a repository
func (d *dedupHandler) Delete(ctx context.Context, repo string, h v1.Hash) error {
	digest := h.String()

	d.mu.Lock()
	defer d.mu.Unlock()

	blob, exists := d.blobs[digest]
	if !exists {
		return errNotFound
	}

	blob.mu.Lock()
	defer blob.mu.Unlock()

	if !blob.repos[repo] {
		return errNotFound
	}

	// Remove repo reference
	delete(blob.repos, repo)
	atomic.AddInt64(&blob.refCount, -1)
	atomic.AddInt64(&d.totalRefs, -1)

	// If no more references, remove the blob
	if blob.refCount <= 0 {
		delete(d.blobs, digest)
		atomic.AddInt64(&d.totalSize, -int64(len(blob.data)))
		atomic.AddInt64(&d.uniqueBlobs, -1)
	}

	return nil
}

// Stats returns deduplication statistics
func (d *dedupHandler) Stats() (totalSize, uniqueBlobs, totalRefs int64) {
	return atomic.LoadInt64(&d.totalSize),
		atomic.LoadInt64(&d.uniqueBlobs),
		atomic.LoadInt64(&d.totalRefs)
}

// GarbageCollect removes unreferenced blobs
func (d *dedupHandler) GarbageCollect(ctx context.Context) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	removed := 0
	for digest, blob := range d.blobs {
		blob.mu.Lock()
		if blob.refCount <= 0 || len(blob.repos) == 0 {
			delete(d.blobs, digest)
			atomic.AddInt64(&d.totalSize, -int64(len(blob.data)))
			atomic.AddInt64(&d.uniqueBlobs, -1)
			removed++
		}
		blob.mu.Unlock()
	}

	return removed, nil
}

// GetReferencingRepos returns all repositories that reference a blob
func (d *dedupHandler) GetReferencingRepos(h v1.Hash) ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	blob, exists := d.blobs[h.String()]
	if !exists {
		return nil, errNotFound
	}

	blob.mu.RLock()
	defer blob.mu.RUnlock()

	repos := make([]string, 0, len(blob.repos))
	for repo := range blob.repos {
		repos = append(repos, repo)
	}

	return repos, nil
}

// dedupDiskHandler implements disk-based storage with deduplication
type dedupDiskHandler struct {
	*diskHandler
	refs map[string]map[string]bool // digest -> repos mapping
	mu   sync.RWMutex
}

// NewDedupDiskHandler creates a new disk-based deduplicated blob handler
func NewDedupDiskHandler(dir string) (BlobHandler, error) {
	base := NewDiskBlobHandler(dir)

	return &dedupDiskHandler{
		diskHandler: base.(*diskHandler),
		refs:        make(map[string]map[string]bool),
	}, nil
}

func (d *dedupDiskHandler) Stat(ctx context.Context, repo string, h v1.Hash) (int64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check if repo has access to this blob
	if repos, ok := d.refs[h.String()]; ok {
		if !repos[repo] {
			return 0, errNotFound
		}
	} else {
		return 0, errNotFound
	}

	return d.diskHandler.Stat(ctx, repo, h)
}

func (d *dedupDiskHandler) Get(ctx context.Context, repo string, h v1.Hash) (io.ReadCloser, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check if repo has access to this blob
	if repos, ok := d.refs[h.String()]; ok {
		if !repos[repo] {
			return nil, errNotFound
		}
	} else {
		return nil, errNotFound
	}

	return d.diskHandler.Get(ctx, repo, h)
}

func (d *dedupDiskHandler) Put(ctx context.Context, repo string, h v1.Hash, rc io.ReadCloser) error {
	digest := h.String()

	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if blob already exists
	if repos, ok := d.refs[digest]; ok {
		// Just add repo reference
		repos[repo] = true
		return nil
	}

	// Store the blob
	if err := d.diskHandler.Put(ctx, repo, h, rc); err != nil {
		return err
	}

	// Track the reference
	d.refs[digest] = map[string]bool{repo: true}
	return nil
}

func (d *dedupDiskHandler) Delete(ctx context.Context, repo string, h v1.Hash) error {
	digest := h.String()

	d.mu.Lock()
	defer d.mu.Unlock()

	repos, ok := d.refs[digest]
	if !ok {
		return errNotFound
	}

	if !repos[repo] {
		return errNotFound
	}

	// Remove repo reference
	delete(repos, repo)

	// If no more references, delete the blob
	if len(repos) == 0 {
		delete(d.refs, digest)
		return d.diskHandler.Delete(ctx, repo, h)
	}

	return nil
}

// Ensure interfaces are implemented
var (
	_ BlobHandler       = (*dedupHandler)(nil)
	_ BlobStatHandler   = (*dedupHandler)(nil)
	_ BlobPutHandler    = (*dedupHandler)(nil)
	_ BlobDeleteHandler = (*dedupHandler)(nil)
	_ BlobHandler       = (*dedupDiskHandler)(nil)
	_ BlobStatHandler   = (*dedupDiskHandler)(nil)
	_ BlobPutHandler    = (*dedupDiskHandler)(nil)
	_ BlobDeleteHandler = (*dedupDiskHandler)(nil)
)

// DedupStats provides statistics about blob deduplication
type DedupStats struct {
	TotalSize   int64
	UniqueBlobs int64
	TotalRefs   int64
	SpaceSaved  int64
}

// GetDedupStats returns deduplication statistics if the handler supports it
func GetDedupStats(h BlobHandler) (*DedupStats, error) {
	switch handler := h.(type) {
	case *dedupHandler:
		totalSize, uniqueBlobs, totalRefs := handler.Stats()
		avgBlobSize := int64(0)
		if uniqueBlobs > 0 {
			avgBlobSize = totalSize / uniqueBlobs
		}
		spaceSaved := (totalRefs - uniqueBlobs) * avgBlobSize
		return &DedupStats{
			TotalSize:   totalSize,
			UniqueBlobs: uniqueBlobs,
			TotalRefs:   totalRefs,
			SpaceSaved:  spaceSaved,
		}, nil
	default:
		return nil, fmt.Errorf("handler does not support deduplication stats")
	}
}
