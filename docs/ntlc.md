# CLI Tooling Guide

This guide covers the command-line interface for the Nautilus core service and its configuration compiler.

## Core Service (nautilus-core)

The `nautilus-core` is the runtime engine. It manages the proxy, service registry, and configuration watching.

### Configuration
`nautilus-core` can be configured via command-line flags or environment variables.

| Flag | Env Var | Default | Description |
| :--- | :--- | :--- | :--- |
| `--config` | `NAUTILUS_CONFIG` | `nautilus.ntl` | Path to compiled `.ntl` or source `Ntlfile`. |
| `--ntlc` | `NAUTILUS_NTLC` | `ntlc` | Path to the `ntlc` compiler executable. |
| `--services` | `NAUTILUS_SERVICES_DIR` | `/var/run/nautilus/services` | Directory to watch for backend UDS sockets. |
| `--entrypoint-dir` | `NAUTILUS_ENTRYPOINT_DIR` | `/var/run/nautilus/entrypoints` | Directory to create entrypoint UDS sockets. |
| `--entrypoint-count` | `NAUTILUS_ENTRYPOINT_COUNT` | `1` | Number of entrypoint sockets to create. |

### Hot-Reloading
Nautilus automatically tracks changes to your configuration. 
- If `--config` points to an `Ntlfile` (source), it uses the specified `ntlc` binary to re-compile on every save.
- If it points to a `.ntl` file (binary), it simply reloads the state.

---

## Compiler (ntlc)

`ntlc` is the tool used to compile human-readable `Ntlfile` configurations into the binary format required by the core engine.

### Usage
```bash
ntlc [flags]
```

### Flags
| Flag | Default | Description |
| :--- | :--- | :--- |
| `-i` | `Ntlfile` | Input `Ntlfile` path (use `-` for stdin). |
| `-o` | `nautilus.ntl` | Output binary file path. |
| `-check`| `false` | Verify syntax only, without generating output. |

### Exit Codes
- `0`: Success.
- `1`: Compilation or Syntax Error.
