#!/bin/bash
set -e
cd "$(dirname "$0")/.."

cleanup() {
    echo "Stopping all processes..."
    # Kill process group to ensure children die
    kill 0
}
trap cleanup EXIT INT TERM

# Ensure binaries exist
if [ ! -f "./MCP-NG/bin/server" ] || [ ! -f "./MCP-NG/bin/gatekeeper" ]; then
    echo "Binaries not found. Please run ./full_install.sh first."
    exit 1
fi

echo "Starting MCP-NG Stack (Admin Mode)..."
mkdir -p logs

# 1. Server
echo "[1/3] Starting Meta-Server (8080)..."
./MCP-NG/bin/server --http-port=8080 --grpc-port=50050 --no-auth > logs/server_stdout.log 2>&1 &

echo "      Waiting for Server..."
# Loop until /health returns 200 (or at least connects)
until curl -s http://127.0.0.1:8080/health > /dev/null; do
    sleep 1
done
echo "      [OK] Server is ready."

# 2. Gatekeeper
echo "[2/3] Starting Gatekeeper (8082)..."
./MCP-NG/bin/gatekeeper --port=8082 > logs/gatekeeper_stdout.log 2>&1 &

echo "      Waiting for Gatekeeper..."
# Loop until Gatekeeper accepts connections (port 8082 open)
until curl -s http://127.0.0.1:8082 > /dev/null; do
    sleep 1
done
# Slight delay to ensure internal proxy map is ready
sleep 1
echo "      [OK] Gatekeeper is active."

# 3. Admin UI
echo "[3/3] Starting Admin UI..."
cd MCP-NG/admin_ui
poetry run python admin.py &

echo
echo "========================================="
echo "   Admin Stack Running"
echo "========================================="
echo " - Server:      http://127.0.0.1:8080"
echo " - Gatekeeper:  http://127.0.0.1:8082"
echo " - Admin UI:    http://127.0.0.1:7860"
echo
echo "Press Ctrl+C to stop."

wait
