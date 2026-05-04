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

For detailed syntax specifications, built-in middleware, and virtual service listings, please refer to the [Syntax Guide](./docs/syntax.md). 
For CLI usage and core configuration, see the [Tooling Guide](./docs/ntlc.md).

## Docker Support

Nautilus can be deployed using Docker and Docker Compose. This is the recommended way to experience the dynamic UDS proxying in a controlled environment.

### Quick Start with Docker Compose

1. **Build and start the stack**:
   ```bash
   docker compose -f examples/docker-compose.yml up --build
   ```


2. **Test the proxy**:
   The example setup includes a `gateway` (socat) that bridges TCP port `8080` to the Nautilus UDS entrypoint.
   ```bash
   # Test the backend service
   curl http://localhost:8080/

   # Test virtual services
   curl http://localhost:8080/health
   curl http://localhost:8080/debug/services
   ```

3. **Directory Structure for Docker**:
   - `/etc/nautilus`: Configuration files (`.ntl`, `Ntlfile`).
   - `/var/run/nautilus/services`: Backend UDS sockets.
   - `/var/run/nautilus/entrypoints`: Nautilus entrypoint sockets.

## Service Permission

Nautilus uses a strict permission model for Unix Domain Sockets (UDS) to ensure security and service isolation while maintaining high-performance communication.

### Directory Structure & Permissions

| Directory | Permission | Purpose |
| :--- | :--- | :--- |
| `/var/run/nautilus/services` | `1777` (Sticky Bit) | Where backend services place their `.sock` files. |
| `/var/run/nautilus/entrypoints` | `0755` | Where Nautilus creates its entrypoint sockets. |

### Security Model

1.  **Isolation (Sticky Bit)**: The `services` directory has the **Sticky Bit** set (`1777`). This allows any backend service to create its own socket but prevents services from deleting or renaming sockets owned by others.
2.  **Access Control (ACL)**: In production (Docker), Nautilus uses **POSIX ACLs** (Access Control Lists) to automatically grant the `nautilus` user read/write access to any socket created in the `services` directory, even if the socket's owner restricts access to other users.
3.  **Privilege Dropping**: The Nautilus Docker image starts as `root` to initialize these permissions and then immediately drops privileges to a non-root `nautilus` user for execution.

### Backend Implementation Advice

To maintain a secure environment, backend services should:
-   **Recommended Permissions**: Set your socket permissions to `0700` (owner only) or `0770` (owner and group). 
-   **Rely on ACLs**: Do **not** use `0666`. The Default ACL on the `services` directory will automatically grant the `nautilus` user the necessary permissions.
-   **Run as Non-Root**: Ensure backend services run as a dedicated user (not `root`) within their own containers.

## License

This project is licensed under the terms of the LICENSE file.
