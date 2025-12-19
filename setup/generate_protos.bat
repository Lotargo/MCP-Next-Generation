@echo off
setlocal

echo --- Step 1: Verifying and installing required Go tools ---

:: Check for protoc
where protoc >nul 2>nul
if %errorlevel% neq 0 (
    echo Error: 'protoc' compiler not found. Please install the Protocol Buffers compiler.
    exit /b 1
)

echo Installing/updating Go plugins for protoc...
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
if %errorlevel% neq 0 exit /b 1
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
if %errorlevel% neq 0 exit /b 1

echo --- Step 2: Generating Go code from .proto definitions ---

set "PROTO_DIR=MCP-NG\proto"
set "OUT_DIR=mcp-shared-proto"

:: Ensure output directory exists
if not exist "%OUT_DIR%" mkdir "%OUT_DIR%"

protoc ^
    -I "%PROTO_DIR%" ^
    --go_out="%OUT_DIR%" --go_opt=paths=source_relative ^
    --go-grpc_out="%OUT_DIR%" --go-grpc_opt=paths=source_relative ^
    "%PROTO_DIR%\mcp.proto"

if %errorlevel% neq 0 (
    echo Error: Failed to generate Go code.
    exit /b 1
)

echo Code generation completed successfully.

echo --- Step 3: Tidying Go module dependencies ---

pushd mcp-shared-proto
go mod tidy
if %errorlevel% neq 0 (
    echo Error: Failed to tidy mcp-shared-proto.
    popd
    exit /b 1
)
popd

echo Shared proto's Go module dependencies have been tidied.
endlocal
