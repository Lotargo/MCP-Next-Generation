#!/bin/bash
#
# This script ensures reliable and reproducible generation of gRPC code.
# It should be executed from the root directory of the project.

# Exit immediately if a command exits with a non-zero status.
set -e

# Add Go's bin directory to the PATH for this script's execution.
export PATH=$PATH:$(go env GOPATH)/bin

echo "--- Step 1: Verifying and installing required Go tools ---"

# Check for the presence of the protobuf compiler.
if ! command -v protoc &> /dev/null
then
    echo "Error: 'protoc' compiler not found. Please install the Protocol Buffers compiler."
    exit 1
fi

# Install the necessary Go plugins for protoc.
echo "Installing/updating Go plugins for protoc..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

echo "--- Step 2: Generating Go code from .proto definitions ---"

# Define project-specific paths.
PROTO_DIR="MCP-NG/proto"
OUT_DIR="mcp-shared-proto"

# Execute the protoc command.
protoc \
    -I "$PROTO_DIR" \
    --go_out="$OUT_DIR" --go_opt=paths=source_relative \
    --go-grpc_out="$OUT_DIR" --go-grpc_opt=paths=source_relative \
    "$PROTO_DIR/mcp.proto"

echo "Code generation completed successfully."

echo "--- Step 3: Tidying Go module dependencies ---"

# Run 'go mod tidy' to ensure the go.mod and go.sum files are up-to-date.
(cd mcp-shared-proto && go mod tidy)

echo "Shared proto's Go module dependencies have been tidied."
