#!/bin/bash

# Registry Performance Benchmark Suite
# Run various configurations to compare performance

set -e

echo "Registry Performance Benchmark Suite"
echo "==================================="
echo ""

# Create output directory
mkdir -p benchmark_results

# Test 1: In-memory with deduplication (default)
echo "Test 1: In-memory registry with deduplication"
go run benchmark.go -output benchmark_results/inmem_dedup.json

# Test 2: In-memory without deduplication
echo -e "\nTest 2: In-memory registry without deduplication"
go run benchmark.go -dedup=false -output benchmark_results/inmem_nodedup.json

# Test 3: Disk-based with deduplication
echo -e "\nTest 3: Disk-based registry with deduplication"
DISK_DIR=$(mktemp -d)
go run benchmark.go -disk "$DISK_DIR" -output benchmark_results/disk_dedup.json
rm -rf "$DISK_DIR"

# Test 4: Disk-based with streaming blobs
echo -e "\nTest 4: Disk-based registry with streaming blobs"
DISK_DIR=$(mktemp -d)
go run benchmark.go -disk "$DISK_DIR" -streaming -output benchmark_results/disk_streaming.json
rm -rf "$DISK_DIR"

# Test 5: Disk-based with streaming and small threshold
echo -e "\nTest 5: Disk-based registry with streaming (small threshold)"
DISK_DIR=$(mktemp -d)
go run benchmark.go -disk "$DISK_DIR" -streaming -stream-threshold=102400 -output benchmark_results/disk_streaming_small.json
rm -rf "$DISK_DIR"

echo -e "\n\nAll benchmarks completed!"
echo "Results saved in benchmark_results/"
echo ""
echo "To compare results:"
echo "  ls -la benchmark_results/*.json"