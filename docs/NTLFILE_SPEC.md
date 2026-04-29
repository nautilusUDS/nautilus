# Ntlfile Specification

The `Ntlfile` (Nautilus File) is a Domain-Specific Language (DSL) used to define routing rules and middleware chains. It is compiled by `ntlc` into a binary format for optimal performance.

## 1. Rule Syntax

A standard rule follows this format:
```nautifile
[METHOD] [HOST/PATH] [SERVICE_TARGET]
    [MIDDLEWARE_1]
    [MIDDLEWARE_2]
```

### Fields
- **Method**: (Optional) HTTP method (GET, POST, etc.). Use `*` for all. Defaults to `*`.
- **Host/Path**: The routing key. Supports wildcard `*` and expansion `[...]`.
- **Service Target**: The directory name of the service or a virtual service starting with `$`.
- **Middlewares**: Indented lines following a rule. Can be built-in functions starting with `$` or paths to external UDS middlewares.

## 2. Expansion Operators

Nautilus supports powerful Cartesian product expansion using square brackets `[]`.

### Pipe Expansion
`[a|b]` expands to `a` and `b`.
- Example: `api/[v1|v2]` -> `api/v1`, `api/v2`.

### Optional Expansion
`[/|]` can be used to handle trailing slashes.
- Example: `data[/|]` -> `data/`, `data`.

### Nested Expansion
Expansions can be nested to create complex patterns.
- Example: `[[prod|stage].|]api.io` -> `prod.api.io`, `stage.api.io`, `api.io`.

## 3. Built-in Functions (Middlewares)

Built-in middlewares perform common tasks without requiring an external process.

| Function | Arguments | Description |
| :--- | :--- | :--- |
| `$Log` | `(prefix)` | Logs request details to stdout. |
| `$SetHeader` | `(key, value)` | Sets a request header. |
| `$DelHeader` | `(key)` | Deletes a request header. |
| `$PathTrimPrefix`| `(prefix)` | Trims a prefix from the URL path. |
| `$RewritePath` | `(old, new)` | Replaces path segments. |
| `$BasicAuth` | `(user, pass)` | Enforces HTTP Basic Authentication. |
| `$IPAllow` | `(cidr)` | Only allows requests from specific IP ranges. |
| `$RateLimit` | `(req, sec)` | Limits requests per IP per time window. |
| `$Redirect` | `(code, url)` | Returns an HTTP redirect. |

## 4. Virtual Services

Virtual services are special targets that generate responses directly within Nautilus Core.

- `$ok("message")`: Returns 200 OK with a custom message.
- `$echo`: Returns a JSON representation of the request headers and metadata.
- `$err("message")`: Returns 500 Internal Server Error (or 4xx/5xx depending on path) with a message.
- `$health`: Built-in system health check.
- `$metrics`: Prometheus-compatible metrics endpoint.

## 5. Comments

Lines starting with `#` are ignored. Use `#*` to start a block comment that skips until the next blank line.
