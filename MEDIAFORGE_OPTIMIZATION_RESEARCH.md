# Mediaforge: Media Processing Optimization Research

**Research Date:** 2026-01-12
**Researcher:** gastown-51 (polecat)
**Status:** Research Phase - Pre-implementation

---

## Executive Summary

This document provides comprehensive research on optimization opportunities for media processing pipelines. The mediaforge rig is planned but not yet implemented, making this an ideal time to establish best practices and architectural patterns for high-performance media operations.

---

## 1. Performance Bottlenecks Analysis

### Common Media Processing Bottlenecks

#### 1.1 I/O Bottlenecks
| Bottleneck | Impact | Mitigation |
|------------|--------|------------|
| Disk read/write speed | High - can be 50-80% of total time | SSD storage, RAID configurations |
| Network transfer latency | High for distributed processing | CDN usage, local caching |
| Memory bandwidth | Medium - affects large file operations | Streaming processing |
| Format conversion overhead | High - CPU intensive | Hardware acceleration |

#### 1.2 CPU Bottlenecks
- **Codec complexity**: H.265/HEVC encoding is 10x slower than H.264
- **Resolution scaling**: 4K processing requires 4x the compute of 1080p
- **Frame rate conversion**: Interpolation algorithms vary greatly in cost
- **Filter chains**: Each filter stage adds CPU overhead

#### 1.3 Memory Bottlenecks
```
Typical memory requirements for processing:
- 1080p video frame: ~6MB (uncompressed RGB)
- 4K video frame: ~24MB (uncompressed RGB)
- 1-minute 1080p video: ~10GB raw
- Worker pool memory: N workers × frame buffer size
```

### Performance Monitoring Framework

```go
// Recommended metrics collection
type MediaProcessingMetrics struct {
    // Throughput metrics
    FilesProcessed    int64
    TotalBytes        int64
    ProcessingTime    time.Duration

    // Resource metrics
    PeakMemoryMB      int64
    AvgCPUPercent     float64
    DiskUtilization   float64

    // Quality metrics
    FrameDropCount    int64
    ErrorRate         float64

    // Per-stage timing
    DecodeTime        time.Duration
    ProcessingTime    time.Duration
    EncodeTime        time.Duration
}
```

---

## 2. Parallel Processing Strategies

### 2.1 Pipeline Parallelism (Producer-Consumer)

```
Input → [Decode] → [Process] → [Encode] → Output
         Stage1    Stage2      Stage3

Each stage runs concurrently with buffered channels.
Optimal when stages have similar processing times.
```

**Go Implementation Pattern:**
```go
type Pipeline struct {
    decodeQueue   chan MediaFrame
    processQueue  chan ProcessedFrame
    encodeQueue   chan EncodedFrame
    workers       int
}

func (p *Pipeline) Run() {
    // Stage 1: Decode workers
    for i := 0; i < p.workers; i++ {
        go p.decodeWorker()
    }
    // Stage 2: Process workers
    for i := 0; i < p.workers; i++ {
        go p.processWorker()
    }
    // Stage 3: Encode workers
    for i := 0; i < p.workers; i++ {
        go p.encodeWorker()
    }
}
```

### 2.2 Data Parallelism (Map-Reduce)

**Frame-level parallelism:**
- Split video into segments (GOP boundaries)
- Process segments concurrently
- Merge results maintaining codec compatibility

**File-level parallelism:**
- Process multiple files simultaneously
- Ideal for batch operations
- Limited by I/O and CPU resources

### 2.3 Hybrid Approach

```
Batch Processing (File-level)
    │
    ├── File 1 ──→ Pipeline Parallelism
    │              ├── Decode
    │              ├── Process (Frame-level)
    │              └── Encode
    │
    ├── File 2 ──→ Pipeline Parallelism
    │
    └── File 3 ──→ Pipeline Parallelism
```

### 2.4 Worker Pool Configuration

| Media Type | Decode Workers | Process Workers | Encode Workers |
|------------|----------------|-----------------|----------------|
| Image batch | 2 | CPU cores | 1 |
| Video (1080p) | 2 | CPU cores / 2 | 2 |
| Video (4K) | 2 | CPU cores / 4 | 2 |
| Live stream | 1 | CPU cores | 1 |

---

## 3. Caching Strategies

### 3.1 Multi-Layer Cache Architecture

```
┌─────────────────────────────────────────┐
│         Application Layer               │
│  ┌───────────────────────────────────┐  │
│  │   Result Cache (Redis/Memcached)  │  │
│  │   - Completed transformations     │  │
│  │   - TTL: 24-72 hours              │  │
│  └───────────────────────────────────┘  │
│  ┌───────────────────────────────────┐  │
│  │   Intermediate Cache (Local SSD)  │  │
│  │   - Decoded frames                │  │
│  │   - Partial results               │  │
│  │   - TTL: 1-6 hours                │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
┌─────────────────────────────────────────┐
│         Storage Layer                   │
│  ┌───────────────────────────────────┐  │
│  │   Source Cache (CDN/Object Store) │  │
│  │   - Original media files          │  │
│  │   - Permanent                     │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

### 3.2 Cache Key Design

```go
type CacheKey struct {
    SourceHash    string    // Content hash of source
    Operation     string    // "resize", "transcode", "filter"
    Parameters    string    // JSON-encoded params
    Quality       string    // "high", "medium", "low"
}

// Example key: "sha256:abc123:resize:1920x1080:high"
func (k CacheKey) String() string {
    return fmt.Sprintf("sha256:%s:%s:%s:%s",
        k.SourceHash, k.Operation, k.Parameters, k.Quality)
}
```

### 3.3 Cache Invalidation Strategies

| Strategy | Use Case | Pros | Cons |
|----------|----------|------|------|
| TTL-based | General media processing | Simple, predictable | May serve stale content |
| Tag-based | Versioned media | Precise control | Complex implementation |
| Content-hash | Immutable sources | Perfect cache hits | Computation overhead |
| Manual | Critical updates | Full control | Error-prone |

### 3.4 Recommended Cache Settings

```yaml
# Application cache (Redis)
result_cache:
  ttl:
    image_transformations: 72h
    video_transcodes: 168h  # 7 days
    thumbnails: 24h
  max_memory: 4GB
  eviction: allkeys-lru

# Intermediate cache (local SSD)
intermediate_cache:
  ttl: 6h
  max_size: 100GB
  format: "Directory-based with LRU eviction"
```

---

## 4. Format Optimization

### 4.1 Optimal Format Selection Matrix

| Use Case | Recommended Format | Rationale |
|----------|-------------------|-----------|
| Web delivery (video) | H.264/AAC in MP4 | Universal compatibility |
| Web delivery (image) | WebP with JPEG fallback | 25-35% smaller than JPEG |
| Archival (video) | ProRes or DNxHD | Lossless/visually lossless |
| Archival (image) | PNG or TIFF | Lossless compression |
| Streaming | HLS/DASH with H.264 | Adaptive bitrate |
| Thumbnails | WebP or AVIF | Best compression ratio |

### 4.2 Codec Performance Comparison

| Codec | Encoding Speed | Decoding Speed | Compression Ratio | Hardware Accel |
|-------|----------------|----------------|-------------------|----------------|
| H.264 | Baseline | Baseline | Baseline | Yes |
| H.265/HEVC | 10x slower | Similar | 50% better | Yes (newer GPUs) |
| AV1 | 20x slower | Similar | 30% better than HEVC | Emerging |
| VP9 | 2x slower | Similar | Similar to HEVC | Limited |
| AVIF | Fast | Fast | Best for images | No |

### 4.3 Format Conversion Best Practices

```go
// Recommended processing pipeline
type FormatOptimizer struct {
    // Preserve quality settings
    QualityPreservation bool
    TargetFormat        string

    // Intelligent fallback
    FallbackFormats []string
    UserAgent       string // For browser capability detection
}

func (o *FormatOptimizer) Optimize(input Media) (Media, error) {
    // 1. Detect input format and capabilities
    // 2. Select optimal output format based on use case
    // 3. Apply format-specific optimizations
    // 4. Validate output quality
}
```

### 4.4 Container Optimization

- **Fast Start (Moov Atom):** Move metadata to beginning of file for instant playback
- **Chunking:** Use DASH/HLS chunked containers for streaming
- **Metadata Stripping:** Remove unnecessary EXIF/GPS data for privacy
- **Audio Optimization:** Use Opus/AAC for best compression at target quality

---

## 5. Cost Reduction Opportunities

### 5.1 Compute Cost Optimization

| Strategy | Estimated Savings | Implementation Complexity |
|----------|-------------------|---------------------------|
| Intelligent caching | 40-60% | Low |
| Queue prioritization | 15-25% | Medium |
| Spot instance usage | 50-70% | High |
| Hardware acceleration | 30-50% | Medium |
| Format optimization | 20-40% | Low |
| CDN offloading | 30-50% | Low |

### 5.2 Storage Cost Optimization

```
Tiered Storage Strategy:

Hot Data (0-7 days):    SSD/NVMe    - $0.08/GB/month
Warm Data (7-90 days):  Standard HDD - $0.02/GB/month
Cold Data (90+ days):   Glacier/TA  - $0.004/GB/month

Estimated savings: 70% for 90-day retention
```

### 5.3 Network Cost Optimization

- **CDN usage:** Reduce origin bandwidth by 90%+
- **Bitrate ladder optimization:** Eliminate unnecessary renditions
- **Progressive download:** Replace streaming for short-form content
- **Edge processing:** Move transformations to CDN edge locations

### 5.4 Cost Monitoring Dashboard Metrics

```go
type CostMetrics struct {
    // Compute costs
    ProcessingCostPerGB    float64
    CostPerMinuteOfVideo   float64
    CostPerImage           float64

    // Storage costs
    StorageCostDaily       float64
    CacheHitRate           float64

    // Network costs
    CDNTransferCost        float64
    OriginBandwidthCost    float64

    // Efficiency metrics
    CostSavingsFromCache   float64
    OptimizationROI        float64
}
```

---

## 6. Proof of Concept Plan

### Phase 1: Infrastructure Setup (Week 1)
- [ ] Set up mediaforge rig structure
- [ ] Install FFmpeg and required dependencies
- [ ] Configure Redis for caching layer
- [ ] Establish metrics collection framework

### Phase 2: Basic Pipeline (Week 2)
- [ ] Implement single-file processing
- [ ] Add basic image transformations
- [ ] Create video transcoding pipeline
- [ ] Implement simple caching

### Phase 3: Optimization (Week 3-4)
- [ ] Add parallel processing workers
- [ ] Implement multi-layer caching
- [ ] Add hardware acceleration (if available)
- [ ] Implement format optimization logic

### Phase 4: Advanced Features (Week 5-6)
- [ ] Streaming pipeline support
- [ ] Batch processing optimization
- [ ] Cost monitoring and alerts
- [ ] Performance benchmarking suite

---

## 7. Recommended Technology Stack

### Core Dependencies

| Component | Technology | Justification |
|-----------|------------|---------------|
| Video Processing | FFmpeg | Industry standard, extensive codec support |
| Image Processing | bimg (libvips) | Fast, memory-efficient |
| GPU Acceleration | NVENC/CUDA | 10-20x faster encoding |
| Caching Layer | Redis | Fast, distributed, TTL support |
| Message Queue | Gas Town Mail | Integrated with our system |
| Metrics | Prometheus | Industry-standard monitoring |
| Storage | S3-compatible | Portable, scalable |

### Go Libraries

```go
// Recommended packages
import (
    "github.com/gabriel-vasile/mimetype" // MIME detection
    "github.com/disintegration/imaging"  // Image processing
    "github.com/go-redis/redis/v8"       // Redis client
    "github.com/prometheus/client_golang" // Metrics
)
```

---

## 8. Performance Targets

### Baseline Performance Goals

| Operation | Target | Baseline (unoptimized) |
|-----------|--------|------------------------|
| Image resize (1MP) | < 100ms | ~500ms |
| Video transcode (1min 1080p) | < 30s | ~120s |
| Thumbnail generation | < 50ms | ~200ms |
| Batch processing (100 images) | < 5s | ~30s |
| Cache hit retrieval | < 10ms | N/A |

### Resource Utilization Targets

- **CPU Utilization:** 70-85% (headroom for spikes)
- **Memory Efficiency:** < 2GB per worker
- **Cache Hit Rate:** > 60% for repeated operations
- **Error Rate:** < 0.1% (1 per 1000 operations)

---

## 9. Next Steps

1. **Review and approve** this research document
2. **Create mediaforge rig** with recommended structure
3. **Implement Phase 1** of PoC plan
4. **Establish benchmarks** for validation
5. **Iterate based on** real-world usage patterns

---

## Appendix: Code Examples

### A.1 Basic Pipeline Skeleton

```go
package mediaforge

import (
    "context"
    "fmt"
)

type MediaFile struct {
    Path     string
    MIME     string
    Metadata map[string]interface{}
}

type ProcessingJob struct {
    Input    MediaFile
    Output   MediaFile
    Options  ProcessingOptions
}

type ProcessingOptions struct {
    // Output format
    Format string
    Quality int

    // Transformations
    Width    int
    Height   int
    Duration int // for video

    // Performance
    EnableCache bool
    Workers     int
}

type Processor struct {
    cache    Cache
    metrics  MetricsCollector
    workers  int
}

func (p *Processor) Process(ctx context.Context, job ProcessingJob) error {
    // Check cache first
    if job.Options.EnableCache {
        if cached, err := p.cache.Get(job); err == nil {
            return p.writeOutput(cached, job.Output)
        }
    }

    // Process the media
    result, err := p.processMedia(ctx, job)
    if err != nil {
        return fmt.Errorf("processing failed: %w", err)
    }

    // Cache the result
    if job.Options.EnableCache {
        p.cache.Set(job, result)
    }

    return p.writeOutput(result, job.Output)
}
```

### A.2 Worker Pool Implementation

```go
func (p *Processor) processMedia(ctx context.Context, job ProcessingJob) ([]byte, error) {
    // Determine processing strategy based on media type
    switch job.Input.MIME {
    case "image/jpeg", "image/png":
        return p.processImage(ctx, job)
    case "video/mp4", "video/webm":
        return p.processVideo(ctx, job)
    default:
        return nil, fmt.Errorf("unsupported media type: %s", job.Input.MIME)
    }
}

func (p *Processor) processImage(ctx context.Context, job ProcessingJob) ([]byte, error) {
    // Use imaging library for efficient processing
    img, err := imaging.Open(job.Input.Path)
    if err != nil {
        return nil, err
    }

    // Apply transformations
    dst := imaging.Resize(img, job.Options.Width, job.Options.Height,
        imaging.Lanczos)

    // Encode output
    var buf bytes.Buffer
    err = imaging.Encode(&buf, dst, imaging.JPEGQuality(job.Options.Quality))
    return buf.Bytes(), err
}
```

---

**Document Version:** 1.0
**Status:** Ready for Review
**Next Review:** After PoC Phase 1 completion
