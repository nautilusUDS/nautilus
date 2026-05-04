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

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /bin/nautilus-core /usr/local/bin/nautilus-core
COPY --from=builder /bin/ntlc /usr/local/bin/ntlc

# Create directories for config and UDS sockets
RUN mkdir -p /etc/nautilus /var/run/nautilus/services /var/run/nautilus/entrypoints

# Set permissions
RUN chmod -R 777 /var/run/nautilus

# Define default volume for sockets (Physical isolation path)
VOLUME ["/var/run/nautilus"]

# Default environment variables
ENV NAUTILUS_CONFIG=/etc/nautilus/nautilus.ntl
ENV NAUTILUS_NTLC=ntlc
ENV NAUTILUS_SERVICES_DIR=/var/run/nautilus/services
ENV NAUTILUS_ENTRYPOINT_DIR=/var/run/nautilus/entrypoints

# Expose potential ports (though core uses UDS, it might expose metrics or entrypoints)
# Entrypoints are usually UDS too in this design, but users might map them.

# Set Core as the entrypoint
ENTRYPOINT ["nautilus-core"]
