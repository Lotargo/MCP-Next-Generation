#!/bin/bash
# This script is intended to be run from the project root.
# The calling process (e.g., test client, LM Studio config) is responsible
# for setting the correct working directory.

echo "Starting MCP-NG Meta-Server..." >&2
./MCP-NG/bin/server "$@"
