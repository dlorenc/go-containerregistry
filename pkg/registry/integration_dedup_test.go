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

package registry_test

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func TestDedupIntegration(t *testing.T) {
	// Create a registry with deduplication enabled
	s := httptest.NewServer(registry.New(registry.WithDeduplication()))
	defer s.Close()

	// Create a base image
	img1, err := random.Image(1024, 3)
	if err != nil {
		t.Fatalf("Failed to create image: %v", err)
	}

	// Push the same image to multiple repositories
	repos := []string{"repo1", "repo2", "repo3"}
	for _, repo := range repos {
		ref, err := name.ParseReference(fmt.Sprintf("%s/%s:latest", s.URL[7:], repo))
		if err != nil {
			t.Fatalf("Failed to parse reference: %v", err)
		}

		if err := remote.Write(ref, img1); err != nil {
			t.Fatalf("Failed to push image to %s: %v", repo, err)
		}
	}

	// Create images that share layers with the first image
	newLayer, err := random.Layer(1024, "new-layer")
	if err != nil {
		t.Fatalf("Failed to create new layer: %v", err)
	}
	
	img2, err := mutate.AppendLayers(img1, newLayer)
	if err != nil {
		t.Fatalf("Failed to create img2: %v", err)
	}

	ref2, err := name.ParseReference(fmt.Sprintf("%s/repo4:latest", s.URL[7:]))
	if err != nil {
		t.Fatalf("Failed to parse reference: %v", err)
	}

	if err := remote.Write(ref2, img2); err != nil {
		t.Fatalf("Failed to push img2: %v", err)
	}

	// Verify all images can be pulled back
	for _, repo := range append(repos, "repo4") {
		ref, err := name.ParseReference(fmt.Sprintf("%s/%s:latest", s.URL[7:], repo))
		if err != nil {
			t.Fatalf("Failed to parse reference: %v", err)
		}

		pulledImg, err := remote.Image(ref)
		if err != nil {
			t.Fatalf("Failed to pull image from %s: %v", repo, err)
		}

		// Verify the image is valid
		if _, err := pulledImg.Digest(); err != nil {
			t.Errorf("Failed to get digest for %s: %v", repo, err)
		}
	}

	// In a real scenario, we would access the registry's internal blob handler
	// to verify deduplication stats, but that requires exposing internals
}

func BenchmarkDedupVsNormal(b *testing.B) {
	// Benchmark deduplication performance vs normal storage
	
	b.Run("Normal", func(b *testing.B) {
		s := httptest.NewServer(registry.New())
		defer s.Close()
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkPushImage(b, s.URL)
		}
	})
	
	b.Run("Dedup", func(b *testing.B) {
		s := httptest.NewServer(registry.New(registry.WithDeduplication()))
		defer s.Close()
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkPushImage(b, s.URL)
		}
	})
}

func benchmarkPushImage(b *testing.B, registryURL string) {
	img, err := random.Image(1024, 3)
	if err != nil {
		b.Fatalf("Failed to create image: %v", err)
	}
	
	ref, err := name.ParseReference(fmt.Sprintf("%s/test:v%d", registryURL[7:], b.N))
	if err != nil {
		b.Fatalf("Failed to parse reference: %v", err)
	}
	
	if err := remote.Write(ref, img); err != nil {
		b.Fatalf("Failed to push image: %v", err)
	}
}