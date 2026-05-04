FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /bin/backend ./examples/backend/main.go

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /bin/backend /usr/local/bin/backend
ENTRYPOINT ["backend"]
