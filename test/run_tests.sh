#!/bin/bash
set -e

echo "[1/4] Building and starting Docker containers..."
docker compose -f test/docker-compose.test.yml up -d --build

echo "[2/4] Waiting for services to be ready..."
# Wait up to 30 seconds
count=0
until [ "$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/debug/health)" == "200" ]; do
    count=$((count+1))
    if [ $count -ge 15 ]; then
        echo "[ERROR] Timeout waiting for services."
        docker compose -f test/docker-compose.test.yml logs
        exit 1
    fi
    echo "Waiting for http://localhost:8080/debug/health (Status: $(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/debug/health))..."
    sleep 2
done

echo "[3/4] Running integration tests..."
go test -v ./test/integration_test.go

echo "[4/4] Cleaning up (Removing containers, networks, and images)..."
docker compose -f test/docker-compose.test.yml down --rmi local -v
