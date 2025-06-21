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
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func TestStreamingBlobHandler(t *testing.T) {
	tmpDir := t.TempDir()
	memThreshold := int64(1024) // 1KB threshold for testing
	
	handler, err := NewStreamingBlobHandler(tmpDir, memThreshold)
	if err != nil {
		t.Fatalf("Failed to create streaming handler: %v", err)
	}

	ctx := context.Background()

	t.Run("SmallBlobInMemory", func(t *testing.T) {
		// Create a small blob (< threshold)
		smallData := []byte("small blob data")
		h, _, _ := v1.SHA256(bytes.NewReader(smallData))
		
		// Put the blob
		if err := handler.(BlobPutHandler).Put(ctx, "test", h, io.NopCloser(bytes.NewReader(smallData))); err != nil {
			t.Fatalf("Failed to put small blob: %v", err)
		}

		// Verify it's not on disk
		diskPath := filepath.Join(tmpDir, h.Algorithm, h.Hex)
		if _, err := os.Stat(diskPath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Small blob should not be on disk, but found at %s", diskPath)
		}

		// Get the blob
		rc, err := handler.Get(ctx, "test", h)
		if err != nil {
			t.Fatalf("Failed to get small blob: %v", err)
		}
		defer rc.Close()

		got, _ := io.ReadAll(rc)
		if !bytes.Equal(got, smallData) {
			t.Errorf("Got %q, want %q", got, smallData)
		}

		// Stat the blob
		size, err := handler.(BlobStatHandler).Stat(ctx, "test", h)
		if err != nil {
			t.Fatalf("Failed to stat small blob: %v", err)
		}
		if size != int64(len(smallData)) {
			t.Errorf("Got size %d, want %d", size, len(smallData))
		}
	})

	t.Run("LargeBlobOnDisk", func(t *testing.T) {
		// Create a large blob (> threshold)
		largeData := make([]byte, memThreshold+100)
		rand.Read(largeData)
		h, _, _ := v1.SHA256(bytes.NewReader(largeData))
		
		// Put the blob
		if err := handler.(BlobPutHandler).Put(ctx, "test", h, io.NopCloser(bytes.NewReader(largeData))); err != nil {
			t.Fatalf("Failed to put large blob: %v", err)
		}

		// Verify it's on disk
		diskPath := filepath.Join(tmpDir, h.Algorithm, h.Hex)
		if _, err := os.Stat(diskPath); err != nil {
			t.Errorf("Large blob should be on disk at %s, but got error: %v", diskPath, err)
		}

		// Get the blob
		rc, err := handler.Get(ctx, "test", h)
		if err != nil {
			t.Fatalf("Failed to get large blob: %v", err)
		}
		defer rc.Close()

		got, _ := io.ReadAll(rc)
		if !bytes.Equal(got, largeData) {
			t.Errorf("Got different data than expected for large blob")
		}

		// Stat the blob
		size, err := handler.(BlobStatHandler).Stat(ctx, "test", h)
		if err != nil {
			t.Fatalf("Failed to stat large blob: %v", err)
		}
		if size != int64(len(largeData)) {
			t.Errorf("Got size %d, want %d", size, len(largeData))
		}
	})

	t.Run("ExactThresholdBlob", func(t *testing.T) {
		// Create a blob exactly at threshold
		exactData := make([]byte, memThreshold)
		rand.Read(exactData)
		h, _, _ := v1.SHA256(bytes.NewReader(exactData))
		
		// Put the blob
		if err := handler.(BlobPutHandler).Put(ctx, "test", h, io.NopCloser(bytes.NewReader(exactData))); err != nil {
			t.Fatalf("Failed to put exact threshold blob: %v", err)
		}

		// Should be in memory (not exceeding threshold)
		diskPath := filepath.Join(tmpDir, h.Algorithm, h.Hex)
		if _, err := os.Stat(diskPath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Exact threshold blob should be in memory, but found on disk at %s", diskPath)
		}
	})

	t.Run("DeleteMemoryBlob", func(t *testing.T) {
		// Create and put a small blob
		smallData := []byte("delete me from memory")
		h, _, _ := v1.SHA256(bytes.NewReader(smallData))
		
		if err := handler.(BlobPutHandler).Put(ctx, "test", h, io.NopCloser(bytes.NewReader(smallData))); err != nil {
			t.Fatalf("Failed to put blob: %v", err)
		}

		// Delete it
		if err := handler.(BlobDeleteHandler).Delete(ctx, "test", h); err != nil {
			t.Fatalf("Failed to delete memory blob: %v", err)
		}

		// Verify it's gone
		_, err := handler.Get(ctx, "test", h)
		if !errors.Is(err, errNotFound) {
			t.Errorf("Expected errNotFound, got %v", err)
		}
	})

	t.Run("DeleteDiskBlob", func(t *testing.T) {
		// Create and put a large blob
		largeData := make([]byte, memThreshold+100)
		rand.Read(largeData)
		h, _, _ := v1.SHA256(bytes.NewReader(largeData))
		
		if err := handler.(BlobPutHandler).Put(ctx, "test", h, io.NopCloser(bytes.NewReader(largeData))); err != nil {
			t.Fatalf("Failed to put blob: %v", err)
		}

		// Delete it
		if err := handler.(BlobDeleteHandler).Delete(ctx, "test", h); err != nil {
			t.Fatalf("Failed to delete disk blob: %v", err)
		}

		// Verify it's gone from disk
		diskPath := filepath.Join(tmpDir, h.Algorithm, h.Hex)
		if _, err := os.Stat(diskPath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Blob should be deleted from disk, but still exists at %s", diskPath)
		}

		// Verify Get returns not found
		_, err := handler.Get(ctx, "test", h)
		if !errors.Is(err, errNotFound) {
			t.Errorf("Expected errNotFound, got %v", err)
		}
	})

	t.Run("NonExistentBlob", func(t *testing.T) {
		h := v1.Hash{Algorithm: "sha256", Hex: "0000000000000000000000000000000000000000000000000000000000000000"}
		
		_, err := handler.Get(ctx, "test", h)
		if !errors.Is(err, errNotFound) {
			t.Errorf("Expected errNotFound for non-existent blob, got %v", err)
		}

		_, err = handler.(BlobStatHandler).Stat(ctx, "test", h)
		if !errors.Is(err, errNotFound) {
			t.Errorf("Expected errNotFound for non-existent blob stat, got %v", err)
		}

		err = handler.(BlobDeleteHandler).Delete(ctx, "test", h)
		if !errors.Is(err, errNotFound) {
			t.Errorf("Expected errNotFound for deleting non-existent blob, got %v", err)
		}
	})
}

func TestStreamingBlobHandlerConcurrent(t *testing.T) {
	tmpDir := t.TempDir()
	memThreshold := int64(1024)
	
	handler, err := NewStreamingBlobHandler(tmpDir, memThreshold)
	if err != nil {
		t.Fatalf("Failed to create streaming handler: %v", err)
	}

	ctx := context.Background()

	// Test concurrent operations
	const numOps = 50
	errCh := make(chan error, numOps*3) // put, get, delete for each

	for i := 0; i < numOps; i++ {
		i := i
		go func() {
			// Create unique data for each goroutine
			var data []byte
			if i%2 == 0 {
				// Small blob
				data = []byte(fmt.Sprintf("small blob %d", i))
			} else {
				// Large blob
				data = make([]byte, memThreshold+100)
				rand.Read(data)
			}
			
			h, _, _ := v1.SHA256(bytes.NewReader(data))
			
			// Put
			if err := handler.(BlobPutHandler).Put(ctx, "test", h, io.NopCloser(bytes.NewReader(data))); err != nil {
				errCh <- fmt.Errorf("put %d: %w", i, err)
				return
			}
			
			// Get
			rc, err := handler.Get(ctx, "test", h)
			if err != nil {
				errCh <- fmt.Errorf("get %d: %w", i, err)
				return
			}
			got, _ := io.ReadAll(rc)
			rc.Close()
			
			if !bytes.Equal(got, data) {
				errCh <- fmt.Errorf("data mismatch for blob %d", i)
				return
			}
			
			// Delete
			if err := handler.(BlobDeleteHandler).Delete(ctx, "test", h); err != nil {
				errCh <- fmt.Errorf("delete %d: %w", i, err)
				return
			}
			
			errCh <- nil
		}()
	}

	// Wait for all operations
	for i := 0; i < numOps; i++ {
		if err := <-errCh; err != nil {
			t.Error(err)
		}
	}
}

func BenchmarkStreamingBlobHandler(b *testing.B) {
	tmpDir := b.TempDir()
	memThreshold := int64(1024 * 1024) // 1MB threshold
	
	handler, err := NewStreamingBlobHandler(tmpDir, memThreshold)
	if err != nil {
		b.Fatalf("Failed to create streaming handler: %v", err)
	}

	ctx := context.Background()

	b.Run("SmallBlobMemory", func(b *testing.B) {
		data := make([]byte, 1024) // 1KB
		rand.Read(data)
		h, _, _ := v1.SHA256(bytes.NewReader(data))
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			handler.(BlobPutHandler).Put(ctx, "test", h, io.NopCloser(bytes.NewReader(data)))
			rc, _ := handler.Get(ctx, "test", h)
			io.Copy(io.Discard, rc)
			rc.Close()
			handler.(BlobDeleteHandler).Delete(ctx, "test", h)
		}
	})

	b.Run("LargeBlobDisk", func(b *testing.B) {
		data := make([]byte, 2*1024*1024) // 2MB
		rand.Read(data)
		h, _, _ := v1.SHA256(bytes.NewReader(data))
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			handler.(BlobPutHandler).Put(ctx, "test", h, io.NopCloser(bytes.NewReader(data)))
			rc, _ := handler.Get(ctx, "test", h)
			io.Copy(io.Discard, rc)
			rc.Close()
			handler.(BlobDeleteHandler).Delete(ctx, "test", h)
		}
	})
}