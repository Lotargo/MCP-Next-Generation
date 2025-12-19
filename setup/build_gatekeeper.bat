@echo off
setlocal

cd /d "%~dp0.."

echo Building Admin Gatekeeper...
go build -o MCP-NG\bin\gatekeeper.exe .\MCP-NG\cmd\gatekeeper
if errorlevel 1 (
    echo Build failed!
    exit /b 1
)

echo Build complete. Binary is in MCP-NG\bin\gatekeeper.exe
