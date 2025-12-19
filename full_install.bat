@echo off
set LOG_FILE=install_full.log
del %LOG_FILE% 2>nul

echo ==============================================
echo    MCP-NG Full Stack Installation
echo    Date: %DATE% %TIME%
echo ==============================================
echo.

echo [1/4] Building Core Server ^& Tools...
echo [1/4] Building Core Server ^& Tools... >> %LOG_FILE%
call .\setup\build_windows.bat >> %LOG_FILE% 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Server Build Failed. Check %LOG_FILE%
    exit /b 1
) else (
    echo [OK] Server Build Successful
)

echo.
echo [2/4] Building Gatekeeper...
echo [2/4] Building Gatekeeper... >> %LOG_FILE%
call .\setup\build_gatekeeper.bat >> %LOG_FILE% 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Gatekeeper Build Failed. Check %LOG_FILE%
    exit /b 1
) else (
    echo [OK] Gatekeeper Build Successful
)

echo.
echo [3/4] Installing Admin UI Dependencies...
echo [3/4] Installing Admin UI Dependencies... >> %LOG_FILE%
call .\setup\build_admin.bat >> %LOG_FILE% 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Admin UI Setup Failed. Check %LOG_FILE%
    exit /b 1
) else (
    echo [OK] Admin UI Setup Successful
)

echo.
echo [4/4] Launching Observability Stack (Docker)...
echo [4/4] Launching Observability Stack (Docker)... >> %LOG_FILE%
docker compose up -d >> %LOG_FILE% 2>&1
if %errorlevel% neq 0 (
    echo [WARN] Docker Start Failed (Non-Critical)
    set DOCKER_STATUS=FAILED
) else (
    echo [OK] Docker Stack Started
    set DOCKER_STATUS=SUCCESS
)

echo.
echo ==============================================
echo    Installation Complete!
echo ==============================================
echo.
echo Summary:
echo  - Core Server:   INSTALLED
echo  - Gatekeeper:    INSTALLED
echo  - Admin UI:      INSTALLED
if "%DOCKER_STATUS%"=="SUCCESS" (
    echo  - Docker Stack:  RUNNING
) else (
    echo  - Docker Stack:  FAILED (See Warning)
)

echo.
echo ----------------------------------------------
echo Usage Instructions:
echo 1. Start Server:      .\run\run_server.bat
echo 2. Start Gatekeeper:  .\run\run_gatekeeper.bat
echo 3. Start Admin UI:    .\run\run_admin.bat
echo    -^> Open Browser:   http://localhost:7860
echo ----------------------------------------------

if "%DOCKER_STATUS%"=="FAILED" (
    echo.
    echo [WARN] WARNING: Observability Stack ^(Loki/Alloy^) did not start.
    echo    Reason: Docker command failed.
    echo    Action: 1. Ensure Docker Desktop is running.
    echo            2. Check network connection.
    echo            3. Send '%LOG_FILE%' to support.
    echo    You can try starting it manually later: 'docker compose up -d'
)

echo.
echo Log saved to: %LOG_FILE%
pause
