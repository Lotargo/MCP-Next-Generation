@echo off
cd /d "%~dp0.."
echo Starting MCP-NG Meta-Server (Client Mode)...

if not exist "MCP-NG\bin\server.exe" (
    echo Server binary not found. Please run full_install.bat first.
    pause
    exit /b 1
)

start "MCP-NG Server" cmd /k ".\MCP-NG\bin\server.exe --http-port=8080 --grpc-port=50050 --no-auth"
echo Server started in a new window.
pause
