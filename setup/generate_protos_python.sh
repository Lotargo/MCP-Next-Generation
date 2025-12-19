#!/bin/bash
#
# This script generates Python gRPC code from .proto files.
# It should be executed from the root directory of the project.

# Exit immediately if a command exits with a non-zero status.
set -e

echo "--- Step 1: Checking Python dependencies ---"

# Check if grpcio-tools is installed
if ! python -m grpc_tools.protoc --version &> /dev/null
then
    echo "Error: 'grpcio-tools' package not found."
    echo "Please install it using: pip install grpcio-tools"
    exit 1
fi

echo "Dependencies found."

echo "--- Step 2: Defining paths ---"

# Directory with our main .proto file
PROTO_DIR="MCP-NG/proto"
# Output directory for the generated Python code
OUTPUT_DIR="mcp-shared-proto-py/mcp_shared_proto_py"


echo "--- Step 3: Generating Python code from .proto ---"

# Execute the generation command
python -m grpc_tools.protoc \
    -I="$PROTO_DIR" \
    --python_out="$OUTPUT_DIR" \
    --grpc_python_out="$OUTPUT_DIR" \
    "$PROTO_DIR/mcp.proto"

echo "Python code generation completed successfully."
echo "Files 'mcp_pb2.py' and 'mcp_pb2_grpc.py' in '$OUTPUT_DIR' have been updated."
