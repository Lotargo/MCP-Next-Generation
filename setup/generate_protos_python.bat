@echo off
setlocal

echo --- Step 1: Checking Python dependencies ---

python -m grpc_tools.protoc --version >nul 2>nul
if %errorlevel% neq 0 (
    echo Error: 'grpcio-tools' package not found.
    echo Please install it using: pip install grpcio-tools
    exit /b 1
)

echo Dependencies found.

echo --- Step 2: Defining paths ---

set "PROTO_DIR=MCP-NG\proto"
set "OUTPUT_DIR=mcp-shared-proto-py\mcp_shared_proto_py"

echo --- Step 3: Generating Python code from .proto ---

python -m grpc_tools.protoc ^
    -I="%PROTO_DIR%" ^
    --python_out="%OUTPUT_DIR%" ^
    --grpc_python_out="%OUTPUT_DIR%" ^
    "%PROTO_DIR%\mcp.proto"

if %errorlevel% neq 0 (
    echo Error: Failed to generate Python code.
    exit /b 1
)

echo Python code generation completed successfully.
echo Files 'mcp_pb2.py' and 'mcp_pb2_grpc.py' in '%OUTPUT_DIR%' have been updated.
endlocal
