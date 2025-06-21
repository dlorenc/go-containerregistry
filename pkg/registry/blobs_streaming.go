// Copyright 2025 Google LLC All Rights Reserved.
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
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// streamingHandler implements BlobHandler with automatic routing of large blobs to disk
type streamingHandler struct {
	memThreshold int64 // Blobs larger than this go to disk
	memStore     map[string][]byte
	diskStore    string // Directory for large blobs
	mu           sync.RWMutex
}

// NewStreamingBlobHandler creates a new streaming blob handler that automatically
// routes large blobs to disk storage
func NewStreamingBlobHandler(diskDir string, memThreshold int64) (BlobHandler, error) {
	if err := os.MkdirAll(diskDir, 0755); err != nil {
		return nil, err
	}
	return &streamingHandler{
		memThreshold: memThreshold,
		memStore:     make(map[string][]byte),
		diskStore:    diskDir,
	}, nil
}

func (s *streamingHandler) diskPath(h v1.Hash) string {
	return filepath.Join(s.diskStore, h.Algorithm, h.Hex)
}

func (s *streamingHandler) Stat(_ context.Context, _ string, h v1.Hash) (int64, error) {
	// Check memory first
	s.mu.RLock()
	if data, ok := s.memStore[h.String()]; ok {
		s.mu.RUnlock()
		return int64(len(data)), nil
	}
	s.mu.RUnlock()

	// Check disk
	fi, err := os.Stat(s.diskPath(h))
	if errors.Is(err, os.ErrNotExist) {
		return 0, errNotFound
	} else if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

func (s *streamingHandler) Get(_ context.Context, _ string, h v1.Hash) (io.ReadCloser, error) {
	// Check memory first
	s.mu.RLock()
	if data, ok := s.memStore[h.String()]; ok {
		s.mu.RUnlock()
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	s.mu.RUnlock()

	// Check disk
	f, err := os.Open(s.diskPath(h))
	if errors.Is(err, os.ErrNotExist) {
		return nil, errNotFound
	}
	return f, err
}

func (s *streamingHandler) Put(_ context.Context, _ string, h v1.Hash, rc io.ReadCloser) error {
	defer rc.Close()

	// Use TeeReader to check size while reading
	buf := &bytes.Buffer{}
	tee := io.TeeReader(rc, buf)

	// Read up to threshold + 1 to determine storage location
	lr := io.LimitReader(tee, s.memThreshold+1)
	n, _ := io.Copy(io.Discard, lr)

	if n > s.memThreshold {
		// Stream to disk
		if err := os.MkdirAll(filepath.Join(s.diskStore, h.Algorithm), 0755); err != nil {
			return err
		}

		// Create temp file in same directory to avoid cross-device issues
		f, err := os.CreateTemp(filepath.Join(s.diskStore, h.Algorithm), "upload-*")
		if err != nil {
			return err
		}
		tempPath := f.Name()
		
		// Write the buffered data and remaining stream
		if _, err := io.Copy(f, io.MultiReader(buf, rc)); err != nil {
			f.Close()
			os.Remove(tempPath)
			return err
		}
		
		if err := f.Close(); err != nil {
			os.Remove(tempPath)
			return err
		}

		// Atomically move to final location
		return os.Rename(tempPath, s.diskPath(h))
	}

	// Small blob - read remaining data and keep in memory
	remaining, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	
	data := append(buf.Bytes(), remaining...)
	
	s.mu.Lock()
	s.memStore[h.String()] = data
	s.mu.Unlock()
	
	return nil
}

func (s *streamingHandler) Delete(_ context.Context, _ string, h v1.Hash) error {
	// Try memory first
	s.mu.Lock()
	if _, ok := s.memStore[h.String()]; ok {
		delete(s.memStore, h.String())
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	// Try disk
	err := os.Remove(s.diskPath(h))
	if errors.Is(err, os.ErrNotExist) {
		return errNotFound
	}
	return err
}