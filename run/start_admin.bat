@echo off
setlocal enabledelayedexpansion
cd /d "%~dp0.."

echo =========================================
echo    Starting MCP-NG Stack (Admin Mode)
echo =========================================

if not exist "MCP-NG\bin\server.exe" (
    echo [ERROR] Binaries not found. Please run full_install.bat first.
    pause
    exit /b 1
)

:: 1. Start Server
echo [1/3] Starting Meta-Server (Port 8080)...
start "MCP-NG Server" /MIN cmd /k ".\MCP-NG\bin\server.exe --http-port=8080 --grpc-port=50050 --no-auth"

echo       Waiting for Server to be ready...
:check_server
timeout /t 1 >nul
curl -s http://127.0.0.1:8080/health >nul
if !errorlevel! neq 0 (
    goto check_server
)
echo       [OK] Server is ready.

:: 2. Start Gatekeeper
echo [2/3] Starting Gatekeeper (Port 8082)...
start "MCP-NG Gatekeeper" /MIN cmd /k ".\MCP-NG\bin\gatekeeper.exe --port=8082"

echo       Waiting for Gatekeeper to claim server...
:check_gatekeeper
timeout /t 1 >nul
:: Check if Gatekeeper port is open. Using curl to connect.
:: Exit code 0 means connection successful (even if 404).
curl -s -I http://127.0.0.1:8082 >nul
if !errorlevel! neq 0 (
    goto check_gatekeeper
)
:: Give it a moment to ensure internal proxy map is set
timeout /t 1 >nul
echo       [OK] Gatekeeper is active.

:: 3. Start UI
echo [3/3] Starting Admin UI...
cd MCP-NG\admin_ui
start "MCP-NG Admin UI" cmd /k "poetry run python admin.py"

echo.
echo =========================================
echo    Admin Stack Running
echo =========================================
echo  - Server:      http://127.0.0.1:8080
echo  - Gatekeeper:  http://127.0.0.1:8082
echo  - Admin UI:    http://127.0.0.1:7860
echo.
echo Close the specific windows to stop services.
pause
