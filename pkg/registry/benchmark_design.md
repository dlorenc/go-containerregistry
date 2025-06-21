# Registry Performance Benchmark Design

## Overview

This benchmark suite measures the performance of the crane registry implementation across multiple dimensions:

1. **Upload Performance**: Single and bulk image uploads of various sizes
2. **Download Performance**: Single and bulk image downloads
3. **Concurrent Operations**: Parallel upload/download operations
4. **Memory Usage**: Peak memory consumption during operations
5. **CPU Usage**: CPU utilization patterns
6. **Throughput**: MB/s transfer rates
7. **Latency**: Operation response times

## Key Metrics

### Upload Metrics
- Time to upload single images (small: 1MB, medium: 10MB, large: 100MB, xlarge: 1GB)
- Bulk upload throughput (100 images of various sizes)
- Concurrent upload performance (10, 50, 100 parallel uploads)
- Registry storage efficiency (deduplication effectiveness)

### Download Metrics
- Time to download single images
- Bulk download throughput
- Concurrent download performance
- Cache hit rates (if applicable)

### Resource Metrics
- Peak memory usage during operations
- CPU utilization percentage
- Disk I/O patterns
- Network bandwidth utilization

## Benchmark Scenarios

### 1. Single Image Operations
- Upload and download individual images of varying sizes
- Measure latency and throughput
- Test with different compression levels

### 2. Bulk Operations
- Upload 100 images sequentially
- Download 100 images sequentially
- Measure total time and average throughput

### 3. Concurrent Operations
- 10 concurrent uploads/downloads
- 50 concurrent uploads/downloads
- 100 concurrent uploads/downloads
- Measure contention and scalability

### 4. Mixed Workload
- Simultaneous uploads and downloads
- Realistic usage patterns
- Measure overall system performance

### 5. Deduplication Testing
- Upload images with shared layers
- Measure storage savings
- Test deduplication performance impact

## Implementation Details

### Registry Setup
- Start registry on localhost with configurable port
- Use in-memory or disk-based storage (configurable)
- Enable metrics collection

### Image Generation
- Generate synthetic images with controlled characteristics:
  - Configurable number of layers
  - Configurable layer sizes
  - Option for shared layers (deduplication testing)
  - Various compression levels

### Measurement Framework
- Use Go's built-in benchmarking tools
- Custom metrics collection for detailed analysis
- CSV/JSON output for result processing
- Real-time progress reporting

### Result Analysis
- Statistical analysis (min, max, mean, p50, p95, p99)
- Throughput calculations (MB/s)
- Resource usage graphs
- Comparison between different scenarios

## Expected Output

The benchmark will produce:
1. Detailed performance metrics in JSON format
2. Summary report with key findings
3. CSV files for further analysis
4. Performance regression detection capabilities