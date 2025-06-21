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

package registry_test

import (
	"fmt"
	"log"
	"net/http/httptest"
	"os"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func Example_streamingBlobs() {
	// Create a temporary directory for disk storage
	tmpDir, err := os.MkdirTemp("", "streaming-blobs-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create registry with streaming blob handler
	// Small blobs (< 1MB) stay in memory, large blobs go to disk
	s := httptest.NewServer(registry.New(
		registry.WithStreamingBlobs(tmpDir, 1024*1024), // 1MB threshold
	))
	defer s.Close()
	
	// Use the server URL without the http:// prefix
	u := s.URL[7:] // strip "http://"
	repo := fmt.Sprintf("%s/test", u)

	// Push a small image (will be stored in memory)
	smallImg, err := random.Image(1024, 1) // 1KB image
	if err != nil {
		log.Fatal(err)
	}

	if err := crane.Push(smallImg, repo+"/small:latest"); err != nil {
		log.Fatal(err)
	}

	// Push a large image (will be stored on disk)
	largeLayer, err := random.Layer(2*1024*1024, types.DockerLayer) // 2MB layer
	if err != nil {
		log.Fatal(err)
	}
	
	largeImg, err := mutate.AppendLayers(empty.Image, largeLayer)
	if err != nil {
		log.Fatal(err)
	}

	if err := crane.Push(largeImg, repo+"/large:latest"); err != nil {
		log.Fatal(err)
	}

	// Pull both images back
	pulledSmall, err := crane.Pull(repo + "/small:latest")
	if err != nil {
		log.Fatal(err)
	}

	pulledLarge, err := crane.Pull(repo + "/large:latest")
	if err != nil {
		log.Fatal(err)
	}

	// Verify they work correctly
	if _, err := pulledSmall.Digest(); err != nil {
		log.Fatal(err)
	}
	if _, err := pulledLarge.Digest(); err != nil {
		log.Fatal(err)
	}
	
	fmt.Println("Successfully pulled small image")
	fmt.Println("Successfully pulled large image")
	
	// Check disk storage - should have entries for large image layers
	entries, _ := os.ReadDir(tmpDir)
	hasEntries := len(entries) > 0
	fmt.Printf("Disk storage has entries: %v\n", hasEntries)
	
	// Output:
	// Successfully pulled small image
	// Successfully pulled large image
	// Disk storage has entries: true
}