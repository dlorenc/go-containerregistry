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

	"github.com/google/go-containerregistry/pkg/registry"
)

func ExampleWithDeduplication() {
	// Create a registry with in-memory deduplication
	s := httptest.NewServer(registry.New(registry.WithDeduplication()))
	defer s.Close()
	
	fmt.Printf("Registry with deduplication running at %s\n", s.URL)
	
	// Blobs will now be deduplicated across repositories
	// When the same blob is pushed to multiple repos, it's stored only once
}

func ExampleWithDiskDeduplication() {
	// Create a registry with disk-based deduplication
	s := httptest.NewServer(registry.New(registry.WithDiskDeduplication("/tmp/registry")))
	defer s.Close()
	
	fmt.Printf("Registry with disk deduplication running at %s\n", s.URL)
}

func ExampleGetDedupStats() {
	// Create a registry with deduplication
	// In practice, you would need to access the blob handler through the registry internals
	// to get deduplication statistics
	
	fmt.Println("Deduplication statistics would show:")
	fmt.Println("- Total unique blobs")
	fmt.Println("- Total references")  
	fmt.Println("- Space saved through deduplication")
}