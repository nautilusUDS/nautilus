# Development Guide

This guide explains how to set up the Nautilus development environment and run integration tests.

## 1. Prerequisites

- **Go**: 1.25 or higher.
- **Rust**: 1.80 or higher (for ntl-tentacle sidecar).
- **Docker & Docker Compose**: For integration testing.

## 2. Local Development

### Compiling ntlc
```bash
go build -o ntlc cmd/ntlc/main.go
```

### Running Nautilus Core
```bash
go run cmd/nautilus-core/main.go --config Ntlfile
```

### Building ntl-tentacle
```bash
cd ntl-tentacle
cargo build --release
```

## 3. Integration Testing (Recommended)

The project uses a Docker-based integration suite that mounts source code and compiles it on-the-fly inside containers.

### Run All Tests
```bash
# On Windows
.\test\run_tests.bat

# On Linux/macOS
bash ./test/run_tests.sh
```

### Test Workflow
1. **Source Mounting**: Local source code is mounted into the containers.
2. **On-the-fly Compilation**: Containers compile Go and Rust code upon startup.
3. **Automated Validation**: `test/integration_test.go` executes a suite of HTTP requests against the gateway.
4. **Cleanup**: Containers, networks, and temporary images are removed after testing.

## 4. Code Structure

- `cmd/`: Entrypoints for Core and Compiler.
- `internal/compiler/`: Ntlfile parsing and expansion logic.
- `internal/rtree/`: Performance-optimized Radix Tree implementation.
- `internal/core/`: Ingress handling, middleware execution, and forwarding.
- `ntl-tentacle/`: Rust implementation of the UDS-to-TCP translator.
- `test/`: Integration tests and Docker configurations.

## 5. Coding Standards

- **Go**: Follow `gofmt`. Ensure all new features include unit tests in their respective packages.
- **Rust**: Use `cargo fmt` and `cargo clippy`.
- **UDS First**: Always prefer Unix Domain Sockets for internal communication. Avoid adding TCP listeners to the Core.
