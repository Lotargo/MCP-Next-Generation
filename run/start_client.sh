#!/bin/bash
set -e
cd "$(dirname "$0")/.."

echo "Starting MCP-NG Meta-Server (Client Mode)..."
echo "Using port 8080. Connect your client (e.g. LM Studio) to http://127.0.0.1:8080"

# Ensure binary exists
if [ ! -f "./MCP-NG/bin/server" ]; then
    echo "Server binary not found. Please run ./setup/build.sh first."
    exit 1
fi

./MCP-NG/bin/server --http-port=8080 --grpc-port=50050 --no-auth
