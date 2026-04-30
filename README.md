# Nautilus

High-Performance UDS Routing Engine for Physical Isolation and Edge Computing

Nautilus is a next-generation routing fabric designed to replace the traditional TCP/IP stack with Unix Domain Sockets (UDS). It provides a non-scannable, zero-latency communication infrastructure tailored for high-security environments and resource-constrained edge nodes.

## Why Nautilus?

*   Non-Scannable: By listening only on UDS .sock files, Nautilus is invisible to traditional network port scanners, significantly reducing the attack surface.
*   Zero-Latency Fabric: Bypassing the TCP stack reduces CPU overhead and context switching, delivering near-hardware performance for inter-process communication.
*   Filesystem as Registry: No Etcd or Consul required. Service discovery is natively handled by the directory structure of your host filesystem.
*   Compiled Routing: Rules defined in Ntlfile are compiled into an optimized Radix Tree binary for atomic, zero-downtime updates.

## Key Components

*   Nautilus Core (Go): The data plane forwarding engine with atomic hot-reload and round-robin load balancing.
*   ntlc (Go): The routing compiler that transforms DSL into high-performance binary snapshots.
*   **ntl-tentacle (Rust)**: An async adapter that bridges UDS traffic to legacy TCP services with built-in health probing.

## Quick Start

### 1. Define your routes (Ntlfile)
```nautifile
# Route localhost traffic to nginx service
GET localhost/api/[v1|v2] nginx
    $BasicAuth("admin", "secret")
    $Log("API_ACCESS")

# Catch-all virtual service
$ok("Nautilus is Active")
```

### 2. Compile and Run
```bash
# Build the binary snapshot
ntlc -i Ntlfile -o nautilus.ntl

# Start the core engine
nautilus-core --config nautilus.ntl
```

## Deployment Recommendation

For public-facing deployments, we highly recommend using **Caddy** as the primary entrypoint (Upstream Proxy). 

Caddy complements Nautilus by providing:
- **Automatic TLS**: Built-in ACME support for zero-config HTTPS.
- **Port Normalization**: Cleans Host headers by removing port suffixes before passing to Nautilus.
- **UDS Native Support**: Seamlessly forwards traffic to Nautilus entrypoint sockets.

Example `Caddyfile`:
```caddy
example.com {
    reverse_proxy unix//var/run/nautilus/nautilus-0.sock
}
```

## Documentation

*   [Architecture](docs/ARCHITECTURE.md) - Deep dive into Radix Trees and UDS design.
*   [Ntlfile Specification](docs/NTLFILE_SPEC.md) - Complete syntax and middleware guide.
*   [Development Guide](docs/DEVELOPMENT.md) - Setup, testing, and contribution.

## Integration Testing

Nautilus features a robust Docker-based testing suite that compiles source code on-the-fly:

```bash
# Run the full integration suite
.\test\run_tests.bat  # Windows
bash ./test/run_tests.sh  # Linux/macOS
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
