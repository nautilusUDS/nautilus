@echo off
SETLOCAL

echo [1/4] Building and starting Docker containers...
docker compose -f test/docker-compose.test.yml up -d --build

echo [2/4] Waiting for services to be ready...
rem Wait up to 30 seconds
set count=0
:WAIT
set /a count+=1
if %count% geq 15 (
    echo [ERROR] Timeout waiting for services.
    docker compose -f test/docker-compose.test.yml logs
    exit /b 1
)
curl -s http://localhost:8080/debug/health > nul
if %errorlevel% neq 0 (
    echo Waiting for http://localhost:8080/debug/health...
    timeout /t 2 /nobreak > nul
    goto WAIT
)

echo [3/4] Running integration tests...
go test -v ./test/integration_test.go

echo [4/4] Cleaning up (Removing containers, networks, and images)...
docker compose -f test/docker-compose.test.yml down --rmi local -v

ENDLOCAL
