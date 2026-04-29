# Nautilus Architecture

Nautilus is a high-performance routing engine designed for **Physical Isolation** and **Edge Computing**. It replaces the traditional TCP/IP network stack with **Unix Domain Sockets (UDS)** to provide a non-scannable, zero-latency communication fabric for modern microservices.

## 1. Core Philosophies

### Full-UDS Architecture
Nautilus Core does not listen on any TCP ports. All ingress and egress traffic flows through `.sock` files. This design inherently prevents network-based port scanning and reduces overhead associated with the TCP stack.

### Filesystem as Registry
Unlike traditional systems that rely on Etcd or Consul, Nautilus uses the directory structure of the host filesystem as its service discovery mechanism.
- **Service Name**: Derived from the relative directory path.
- **Node ID**: Derived from the filename of the `.sock` file (without extension).

## 2. Technical Components

### Nautilus Compiler (ntlc)
The compiler transforms human-readable `Ntlfile` rules into an optimized binary snapshot.
- **Radix Tree Construction**: Rules are compiled into a specialized Radix Tree.
- **Reverse Host Indexing**: Hostnames (e.g., `google.com`) are indexed in reverse (`com.google`) to improve prefix matching efficiency.
- **Memory Layout**: The tree uses a flattened `NodePool` and `FragmentPool` to ensure cache locality and zero-allocation lookups.

### Nautilus Core (Go)
The data plane responsible for forwarding traffic.
- **Atomic Hot-Reload**: Detects new `.ntl` snapshots and swaps the routing table instantly using `atomic.Pointer`.
- **Hybrid Watcher**: Combines `fsnotify` events with high-frequency scanning to ensure nodes are discovered immediately while remaining efficient during idle periods.
- **Round-Robin Load Balancing**: Balances traffic across discovered UDS nodes for each service.

### Relay Sidecar (Rust)
A high-performance adapter for legacy TCP services.
- **Bidirectional Copy**: Uses `tokio::io::copy_bidirectional` for zero-copy-like performance.
- **Health Probing**: Only binds the UDS interface if the upstream TCP service is responsive.
- **Concurrency Control**: Manages resources using a global Semaphore pool.

## 3. Request Lifecycle

1. **Ingress**: A request arrives at a Nautilus UDS entrypoint.
2. **Lookup**: The Core reverses the Host header and searches the Radix Tree for a matching node.
3. **Middleware**: Executes a chain of built-in (e.g., `$Log`, `$BasicAuth`) or external UDS middlewares.
4. **Load Balance**: Selects a healthy node path from the Registry.
5. **Forwarding**: The `Forwarder` streams the request to the target UDS path.
6. **Relay (Optional)**: If the target is a Relay, it translates the UDS stream back to a local TCP connection.
