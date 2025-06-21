package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http/httptest"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

var (
	dedup           = flag.Bool("dedup", true, "Enable deduplication")
	diskStorage     = flag.String("disk", "", "Use disk storage (empty for in-memory)")
	streamingBlobs  = flag.Bool("streaming", false, "Use streaming blob storage")
	streamThreshold = flag.Int64("stream-threshold", 1024*1024, "Streaming threshold in bytes")
	outputFile      = flag.String("output", "benchmark_results.json", "Output file for results")
	verbose         = flag.Bool("verbose", false, "Verbose output")
)

type BenchmarkResult struct {
	Name            string        `json:"name"`
	Operations      int           `json:"operations"`
	TotalDuration   time.Duration `json:"total_duration_ns"`
	AvgDuration     time.Duration `json:"avg_duration_ns"`
	MinDuration     time.Duration `json:"min_duration_ns"`
	MaxDuration     time.Duration `json:"max_duration_ns"`
	BytesTransferred int64        `json:"bytes_transferred"`
	ThroughputMBps  float64       `json:"throughput_mbps"`
	Errors          int           `json:"errors"`
}

type BenchmarkSuite struct {
	registry *httptest.Server
	baseURL  string
	results  []BenchmarkResult
	mu       sync.Mutex
}

func NewBenchmarkSuite() (*BenchmarkSuite, error) {
	var opts []registry.Option

	if *dedup {
		if *diskStorage != "" {
			opts = append(opts, registry.WithDiskDeduplication(*diskStorage))
		} else {
			opts = append(opts, registry.WithDeduplication())
		}
	}

	if *streamingBlobs && *diskStorage != "" {
		opts = append(opts, registry.WithStreamingBlobs(*diskStorage, *streamThreshold))
	}

	if *verbose {
		opts = append(opts, registry.Logger(log.New(os.Stdout, "[registry] ", log.LstdFlags)))
	}

	reg := registry.New(opts...)
	server := httptest.NewServer(reg)

	return &BenchmarkSuite{
		registry: server,
		baseURL:  server.URL[7:], // strip "http://"
		results:  make([]BenchmarkResult, 0),
	}, nil
}

func (b *BenchmarkSuite) Close() {
	b.registry.Close()
}

func (b *BenchmarkSuite) recordResult(result BenchmarkResult) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.results = append(b.results, result)
}

func generateImage(sizeMB int, layers int) (v1.Image, error) {
	bytesPerLayer := int64(sizeMB * 1024 * 1024 / layers)
	return random.Image(bytesPerLayer, int64(layers), random.WithSource(rand.NewSource(time.Now().UnixNano())))
}

func generateImageWithSharedLayers(sizeMB int, layers int, sharedLayers []v1.Layer) (v1.Image, error) {
	img := empty.Image
	var err error
	
	// Add shared layers first
	for _, layer := range sharedLayers {
		img, err = mutate.AppendLayers(img, layer)
		if err != nil {
			return nil, err
		}
	}
	
	// Add unique layers
	uniqueLayers := layers - len(sharedLayers)
	if uniqueLayers > 0 {
		bytesPerLayer := int64(sizeMB * 1024 * 1024 / layers / 2) // Half size for unique layers
		for i := 0; i < uniqueLayers; i++ {
			layer, err := random.Layer(bytesPerLayer, types.DockerLayer, random.WithSource(rand.NewSource(time.Now().UnixNano()+int64(i))))
			if err != nil {
				return nil, err
			}
			img, err = mutate.AppendLayers(img, layer)
			if err != nil {
				return nil, err
			}
		}
	}
	
	return img, nil
}

func (b *BenchmarkSuite) benchmarkSingleUpload(imageName string, sizeMB int, layers int) error {
	img, err := generateImage(sizeMB, layers)
	if err != nil {
		return err
	}

	repo := fmt.Sprintf("%s/bench/%s", b.baseURL, imageName)
	ref, err := name.ParseReference(repo)
	if err != nil {
		return err
	}

	start := time.Now()
	err = remote.Write(ref, img, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	duration := time.Since(start)

	result := BenchmarkResult{
		Name:             fmt.Sprintf("upload_single_%dmb_%dlayers", sizeMB, layers),
		Operations:       1,
		TotalDuration:    duration,
		AvgDuration:      duration,
		MinDuration:      duration,
		MaxDuration:      duration,
		BytesTransferred: int64(sizeMB * 1024 * 1024),
		ThroughputMBps:   float64(sizeMB) / duration.Seconds(),
	}

	if err != nil {
		result.Errors = 1
	}

	b.recordResult(result)
	return err
}

func (b *BenchmarkSuite) benchmarkSingleDownload(imageName string, sizeMB int) error {
	repo := fmt.Sprintf("%s/bench/%s", b.baseURL, imageName)
	ref, err := name.ParseReference(repo)
	if err != nil {
		return err
	}

	start := time.Now()
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err == nil {
		// Force full download by reading manifest
		_, err = img.Manifest()
	}
	duration := time.Since(start)

	result := BenchmarkResult{
		Name:             fmt.Sprintf("download_single_%dmb", sizeMB),
		Operations:       1,
		TotalDuration:    duration,
		AvgDuration:      duration,
		MinDuration:      duration,
		MaxDuration:      duration,
		BytesTransferred: int64(sizeMB * 1024 * 1024),
		ThroughputMBps:   float64(sizeMB) / duration.Seconds(),
	}

	if err != nil {
		result.Errors = 1
	}

	b.recordResult(result)
	return err
}

func (b *BenchmarkSuite) benchmarkConcurrentUploads(concurrency int, sizeMB int, count int) error {
	var wg sync.WaitGroup
	errors := int32(0)
	durations := make([]time.Duration, count)
	totalBytes := int64(0)

	sem := make(chan struct{}, concurrency)
	
	totalStart := time.Now()
	
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			
			sem <- struct{}{}
			defer func() { <-sem }()
			
			img, err := generateImage(sizeMB, 3)
			if err != nil {
				atomic.AddInt32(&errors, 1)
				return
			}
			
			repo := fmt.Sprintf("%s/bench/concurrent_%d", b.baseURL, idx)
			ref, err := name.ParseReference(repo)
			if err != nil {
				atomic.AddInt32(&errors, 1)
				return
			}
			
			start := time.Now()
			err = remote.Write(ref, img, remote.WithAuthFromKeychain(authn.DefaultKeychain))
			durations[idx] = time.Since(start)
			
			if err != nil {
				atomic.AddInt32(&errors, 1)
			} else {
				atomic.AddInt64(&totalBytes, int64(sizeMB*1024*1024))
			}
		}(i)
	}
	
	wg.Wait()
	totalDuration := time.Since(totalStart)
	
	// Calculate statistics
	var minDur, maxDur, totalDur time.Duration
	validOps := 0
	for _, d := range durations {
		if d > 0 {
			validOps++
			totalDur += d
			if minDur == 0 || d < minDur {
				minDur = d
			}
			if d > maxDur {
				maxDur = d
			}
		}
	}
	
	avgDur := time.Duration(0)
	if validOps > 0 {
		avgDur = totalDur / time.Duration(validOps)
	}
	
	result := BenchmarkResult{
		Name:             fmt.Sprintf("upload_concurrent_%d_%dmb", concurrency, sizeMB),
		Operations:       count,
		TotalDuration:    totalDuration,
		AvgDuration:      avgDur,
		MinDuration:      minDur,
		MaxDuration:      maxDur,
		BytesTransferred: totalBytes,
		ThroughputMBps:   float64(totalBytes) / 1024 / 1024 / totalDuration.Seconds(),
		Errors:           int(errors),
	}
	
	b.recordResult(result)
	return nil
}

func (b *BenchmarkSuite) benchmarkDeduplication() error {
	// Create shared layers
	sharedLayers := make([]v1.Layer, 2)
	for i := 0; i < 2; i++ {
		layer, err := random.Layer(5*1024*1024, types.DockerLayer, random.WithSource(rand.NewSource(42+int64(i))))
		if err != nil {
			return err
		}
		sharedLayers[i] = layer
	}
	
	// Upload images with shared layers
	totalStart := time.Now()
	for i := 0; i < 10; i++ {
		img, err := generateImageWithSharedLayers(20, 4, sharedLayers)
		if err != nil {
			return err
		}
		
		repo := fmt.Sprintf("%s/bench/dedup_%d", b.baseURL, i)
		ref, err := name.ParseReference(repo)
		if err != nil {
			return err
		}
		
		err = remote.Write(ref, img, remote.WithAuthFromKeychain(authn.DefaultKeychain))
		if err != nil {
			return err
		}
	}
	totalDuration := time.Since(totalStart)
	
	result := BenchmarkResult{
		Name:             "upload_deduplication_test",
		Operations:       10,
		TotalDuration:    totalDuration,
		AvgDuration:      totalDuration / 10,
		BytesTransferred: 10 * 20 * 1024 * 1024, // Logical size
		ThroughputMBps:   float64(10*20) / totalDuration.Seconds(),
		Errors:           0,
	}
	
	b.recordResult(result)
	return nil
}

func (b *BenchmarkSuite) runBenchmarks() error {
	fmt.Println("Starting registry performance benchmarks...")
	fmt.Printf("Registry URL: http://%s\n", b.baseURL)
	fmt.Printf("Deduplication: %v\n", *dedup)
	fmt.Printf("Disk storage: %s\n", *diskStorage)
	fmt.Printf("Streaming blobs: %v\n", *streamingBlobs)
	fmt.Println()

	// Single image uploads
	fmt.Println("Running single image upload benchmarks...")
	sizes := []int{1, 10, 100}
	for _, size := range sizes {
		if err := b.benchmarkSingleUpload(fmt.Sprintf("single_%dmb", size), size, 3); err != nil {
			fmt.Printf("Error in single upload %dMB: %v\n", size, err)
		}
	}

	// Single image downloads
	fmt.Println("Running single image download benchmarks...")
	for _, size := range sizes {
		if err := b.benchmarkSingleDownload(fmt.Sprintf("single_%dmb", size), size); err != nil {
			fmt.Printf("Error in single download %dMB: %v\n", size, err)
		}
	}

	// Concurrent uploads
	fmt.Println("Running concurrent upload benchmarks...")
	concurrencies := []int{10, 50}
	for _, conc := range concurrencies {
		if err := b.benchmarkConcurrentUploads(conc, 10, 50); err != nil {
			fmt.Printf("Error in concurrent upload (concurrency=%d): %v\n", conc, err)
		}
	}

	// Deduplication test
	if *dedup {
		fmt.Println("Running deduplication benchmark...")
		if err := b.benchmarkDeduplication(); err != nil {
			fmt.Printf("Error in deduplication benchmark: %v\n", err)
		}
	}

	return nil
}

func (b *BenchmarkSuite) saveResults() error {
	data, err := json.MarshalIndent(b.results, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(*outputFile, data, 0644)
}

func (b *BenchmarkSuite) printSummary() {
	fmt.Println("\n=== Benchmark Summary ===")
	for _, result := range b.results {
		fmt.Printf("\n%s:\n", result.Name)
		fmt.Printf("  Operations: %d\n", result.Operations)
		fmt.Printf("  Total Duration: %v\n", result.TotalDuration)
		fmt.Printf("  Avg Duration: %v\n", result.AvgDuration)
		fmt.Printf("  Throughput: %.2f MB/s\n", result.ThroughputMBps)
		if result.Errors > 0 {
			fmt.Printf("  Errors: %d\n", result.Errors)
		}
	}
}

func main() {
	flag.Parse()

	// Print system info
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("CPUs: %d\n", runtime.NumCPU())
	fmt.Printf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))
	fmt.Println()

	suite, err := NewBenchmarkSuite()
	if err != nil {
		log.Fatalf("Failed to create benchmark suite: %v", err)
	}
	defer suite.Close()

	if err := suite.runBenchmarks(); err != nil {
		log.Fatalf("Failed to run benchmarks: %v", err)
	}

	if err := suite.saveResults(); err != nil {
		log.Fatalf("Failed to save results: %v", err)
	}

	suite.printSummary()
	fmt.Printf("\nResults saved to: %s\n", *outputFile)
}