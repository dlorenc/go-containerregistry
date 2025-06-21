# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

go-containerregistry is a Go library and set of command-line tools for working with container registries. It provides functionality to read, write, and manipulate OCI images with support for various backends (registries, tarballs, Docker daemon, OCI layout).

## Key Commands

### Development Workflow

```bash
# Run all presubmit checks (required before PRs)
./hack/presubmit.sh

# Run all tests
go test ./...

# Run tests with coverage and race detection
go test -coverprofile=coverage.txt -covermode=atomic -race ./...

# Build everything
go build ./...

# Update generated code and documentation
./hack/update-codegen.sh

# Update dependencies
go mod tidy
go mod vendor
```

### Working with Specific Tests

```bash
# Run a single test
go test -run TestName ./pkg/v1/...

# Run tests for a specific package
go test ./pkg/crane/

# Run integration test scripts
./pkg/name/internal/must_test.sh
./cmd/crane/rebase_test.sh
```

## Architecture

### Core Design Principles

1. **Immutable Resources**: All core types (Image, Layer, ImageIndex) are immutable interfaces
2. **Functional Mutations**: Changes create new views rather than modifying existing ones
3. **Medium-Agnostic**: Same interfaces work with registries, tarballs, daemons, and OCI layouts
4. **Lazy Evaluation**: Operations are deferred until actually needed

### Package Structure

- **pkg/v1/** - Core interfaces and types (Image, Layer, ImageIndex, Descriptor)
- **pkg/v1/remote/** - Registry client implementation
- **pkg/v1/mutate/** - Functions for modifying images (returns new immutable views)
- **pkg/v1/tarball/** - Read/write images as tarballs
- **pkg/v1/daemon/** - Docker daemon integration
- **pkg/v1/layout/** - OCI disk layout support
- **pkg/authn/** - Authentication (keychains, Docker config, k8s)
- **pkg/crane/** - High-level operations library used by CLI
- **cmd/crane/** - Main CLI tool
- **cmd/gcrane/** - Google-specific variant
- **cmd/krane/** - Kubernetes-aware variant

### Key Interfaces

```go
// pkg/v1/image.go
type Image interface {
    Layers() ([]Layer, error)
    MediaType() (types.MediaType, error)
    Size() (int64, error)
    ConfigName() (Hash, error)
    ConfigFile() (*ConfigFile, error)
    RawConfigFile() ([]byte, error)
    Digest() (Hash, error)
    Manifest() (*Manifest, error)
    RawManifest() ([]byte, error)
    LayerByDigest(Hash) (Layer, error)
    LayerByDiffID(Hash) (Layer, error)
}

// pkg/v1/layer.go
type Layer interface {
    Digest() (Hash, error)
    DiffID() (Hash, error)
    Compressed() (io.ReadCloser, error)
    Uncompressed() (io.ReadCloser, error)
    Size() (int64, error)
    MediaType() (types.MediaType, error)
}
```

### Authentication Flow

1. Authentication is handled through the `authn.Keychain` interface
2. Default keychain reads from Docker config files
3. Special keychains exist for GitHub, k8s, and other environments
4. The `remote` package uses keychains to authenticate requests

### Testing Patterns

- Use `pkg/registry` for in-memory test registries
- Mock HTTP responses with `httptest`
- Table-driven tests for comprehensive coverage
- Helper functions in `pkg/v1/validate` for validation

### Common Development Tasks

When adding new functionality:

1. **For new image mutations**: Add to `pkg/v1/mutate/`
2. **For new crane commands**: Add to `cmd/crane/cmd/`
3. **For registry features**: Modify `pkg/v1/remote/`
4. **For auth methods**: Extend `pkg/authn/`

Always ensure:
- New exports have godoc comments
- Tests cover new functionality
- Run `./hack/update-codegen.sh` if adding CLI commands
- Run `./hack/presubmit.sh` before submitting PRs