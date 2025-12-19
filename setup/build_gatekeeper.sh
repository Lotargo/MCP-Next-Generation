#!/bin/bash
set -e

# Move to project root
cd "$(dirname "$0")/.."

echo "Building Admin Gatekeeper..."
go build -o MCP-NG/bin/gatekeeper ./MCP-NG/cmd/gatekeeper

echo "Build complete. Binary is in MCP-NG/bin/gatekeeper"
