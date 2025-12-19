#!/bin/bash
set -e
cd "$(dirname "$0")/../MCP-NG/admin_ui"
echo "Installing Admin UI dependencies..."
poetry install
