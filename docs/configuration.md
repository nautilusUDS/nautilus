# Configuration Guide

Nautilus uses the `Ntlfile` as a DSL configuration file, which is compiled for use by the core engine.

## Compiler (ntlc)

`ntlc` is the tool used to compile human-readable `Ntlfile` configurations into the binary format (`.ntl`) required by the core engine.

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

## Syntax Specification

### Configuration Example (Ntlfile)

The following example demonstrates different column count variations and the use of comments:

```text
# 1. Provide only the service name (Method=*, URL=*/[|*])
# Forwards all traffic to backend-default
backend-default

# 2. Provide URL and service name (Method=*)
# Matches all HTTP methods for /api/*
/api/* api-service

# 3. Full definition (Method, URL, Service)
# Restricts POST requests to /upload/*
POST /upload/* storage-service
    $IPAllow(192.168.0.0/16)  # Restrict to internal network only

# 4. Virtual services and middleware example
GET /admin $health
    $BasicAuth(admin, password123) # Use built-in BasicAuth middleware
    $Log(admin-access)            # Log access requests
```

### Comments
- `#`: Single-line comment.
- `#*`: Block comment (skips until the next blank line).

## Built-in Components

### Built-in Middlewares ($)
| Name | Arguments | Description |
| :--- | :--- | :--- |
| `$SetHeader` | `(key, value)` | Sets a response header. |
| `$DelHeader` | `(key)` | Deletes a header. |
| `$SetHost` | `(host)` | Overwrites the `Host` header. |
| `$PathTrimPrefix`| `(prefix)` | Removes prefix from URL path. |
| `$RewritePath` | `(old, new)` | Replaces pattern in URL path. |
| `$SetQuery` | `(key, value)` | Sets a query parameter. |
| `$BasicAuth` | `(user, pass)` | Basic Authentication. |
| `$IPAllow` | `(cidr)` | Restricts access by CIDR. |
| `$Log` | `(prefix)` | Logs request info. |

### Virtual Services ($)
| Name | Arguments | Description |
| :--- | :--- | :--- |
| `$echo` | - | Returns request info as JSON. |
| `$ok` | `(msg?)` | Returns 200 OK. |
| `$err` | `(code, msg?)` | Returns custom error code/msg. |
| `$health` | - | Synonym for `$ok`. |
| `$metrics` | - | Exposes internal metrics. |
| `$redirect` | `(code, url)` | Performs a redirect. |
| `$json` | `(body?)` | Returns custom JSON response. |
| `$ping` | - | Checks service connectivity. |
