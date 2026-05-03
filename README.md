# Nautilus

English | [中文](README.zh-TW.md)

Nautilus is a dynamic service management and proxy system designed for high-availability request routing and service discovery. It facilitates seamless traffic management through Unix Domain Sockets (UDS) and hot-reloading configurations.

## Core Features

- **Hot-Reloading Configurations**: Automatically tracks changes to `.ntl` or `Ntlfile` configuration files.
- **Dynamic Service Discovery**: Real-time registry management for active services.
- **UDS Proxying**: Efficient request forwarding via Unix Domain Sockets.
- **Graceful Lifecycle Management**: Clean automated setup and teardown of socket listeners and service states.

## Getting Started

### Prerequisites

- Go 1.25.6 or higher.

### Installation

```bash
# Clone the repository
git clone https://github.com/your-repo/nautilus.git
cd nautilus

# Build the core binary
go build -o bin/nautilus-core ./cmd/nautilus-core
```

### Usage

Run the Nautilus core service:

```bash
./bin/nautilus-core --config=my-app.ntl
```

## Configuration

Nautilus uses an `Ntlfile` as a configuration file, which is compiled by the `ntlc` compiler into a binary format (`.ntl`) for hot-reloading by the core engine.

### Configuration Compilation (ntlc)

Use the `ntlc` tool to compile your `Ntlfile` into a binary format readable by the Nautilus core:

```bash
# Basic compilation command
./bin/ntlc -i Ntlfile -o nautilus.ntl
```

### Configuration Example (Ntlfile)

```text
# Basic routing rules
GET /api/v1/users $user-service
    $SetHeader(X-Source, Nautilus)
    $BasicAuth(admin, secret)

POST /upload/* storage-service
    $IPAllow(192.168.1.0/24)
```

For detailed syntax specifications, built-in middleware, and virtual service listings, please refer to the [Configuration Guide](./docs/configuration.md).

## Architecture

- **`cmd/`**: Entry points for the core engine and the `ntlc` compiler.
- **`internal/`**: Core logic including proxying, service registration, and configuration watching.

## License

This project is licensed under the terms of the LICENSE file.
