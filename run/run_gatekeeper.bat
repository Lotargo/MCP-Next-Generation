@echo off
setlocal

cd /d "%~dp0.."

if not exist "MCP-NG\bin\gatekeeper.exe" (
    echo Gatekeeper binary not found. Running build...
    call .\setup\build_gatekeeper.bat
    if errorlevel 1 exit /b 1
)

echo Starting Admin Gatekeeper...
echo Note: Ensure the MCP-NG Meta-Server is already running (e.g. via .\run\run_server.bat)
echo The Gatekeeper will automatically generate an Admin Token and claim the server.

REM Default port 8082
.\MCP-NG\bin\gatekeeper.exe --port=8082
