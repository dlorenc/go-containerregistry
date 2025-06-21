# Registry Performance Benchmark

This benchmark tool measures the performance of the crane registry implementation across multiple dimensions.

## Usage

```bash
go run benchmark.go [flags]
```

### Flags

- `-dedup`: Enable deduplication (default: true)
- `-disk`: Use disk storage path (empty for in-memory)
- `-streaming`: Use streaming blob storage (default: false)
- `-stream-threshold`: Streaming threshold in bytes (default: 1MB)
- `-output`: Output file for results (default: benchmark_results.json)
- `-verbose`: Verbose output (default: false)

### Examples

```bash
# In-memory registry with deduplication
go run benchmark.go

# Disk-based registry with streaming blobs
go run benchmark.go -disk /tmp/registry -streaming

# Without deduplication
go run benchmark.go -dedup=false

# Verbose mode with custom output
go run benchmark.go -verbose -output results.json
```

## Benchmarks Performed

1. **Single Image Upload/Download**
   - Tests with 1MB, 10MB, and 100MB images
   - Measures throughput and latency

2. **Concurrent Operations**
   - 10 and 50 concurrent uploads
   - Tests registry scalability

3. **Deduplication Effectiveness**
   - Uploads images with shared layers
   - Measures storage efficiency

## Output

The benchmark produces:
- JSON file with detailed metrics
- Console summary with key statistics
- Throughput measurements in MB/s
- Min/max/average duration statistics

## Interpreting Results

- **Throughput**: Higher is better (MB/s)
- **Duration**: Lower is better
- **Errors**: Should be 0 for successful runs
- **Deduplication**: Look for reduced storage with shared layers