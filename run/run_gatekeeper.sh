#!/bin/bash
set -e

# Move to project root
cd "$(dirname "$0")/.."

# Ensure bin exists
if [ ! -f "./MCP-NG/bin/gatekeeper" ]; then
    echo "Gatekeeper binary not found. Running build..."
    ./setup/build_gatekeeper.sh
fi

echo "Starting Admin Gatekeeper..."
echo "Note: Ensure the MCP-NG Meta-Server is already running (e.g. via ./run/run_server.sh)"
echo "The Gatekeeper will automatically generate an Admin Token and claim the server."

# Default port 8082
./MCP-NG/bin/gatekeeper --port=8082
