# --- Build Stage ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy dependency manifests
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build Nautilus Core and Compiler
RUN go build -o /bin/nautilus-core ./cmd/nautilus-core
RUN go build -o /bin/ntlc ./cmd/ntlc

# --- Final Stage ---
FROM alpine:3.21

# Install runtime dependencies: su-exec for privilege dropping, acl for fine-grained permissions
RUN apk add --no-cache su-exec acl

# Create non-root user for the service
RUN addgroup -S nautilus && adduser -S nautilus -G nautilus

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /bin/nautilus-core /usr/local/bin/nautilus-core
COPY --from=builder /bin/ntlc /usr/local/bin/ntlc

# Copy entrypoint script
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# Create initial directory structure
RUN mkdir -p /etc/nautilus /var/run/nautilus/services /var/run/nautilus/entrypoints && \
    chown -R nautilus:nautilus /var/run/nautilus /etc/nautilus

# Define default volume for sockets
VOLUME ["/var/run/nautilus"]

# Default environment variables
ENV NAUTILUS_CONFIG=/etc/nautilus/nautilus.ntl
ENV NAUTILUS_NTLC=ntlc
ENV NAUTILUS_SERVICES_DIR=/var/run/nautilus/services
ENV NAUTILUS_ENTRYPOINT_DIR=/var/run/nautilus/entrypoints

# The entrypoint script will handle final permission adjustments and drop privileges
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
