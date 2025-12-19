#!/bin/bash
set -e

LOG_FILE="install_full.log"
rm -f "$LOG_FILE"

# Redirect output to log file AND stdout
exec > >(tee -a "$LOG_FILE") 2>&1

echo "=============================================="
echo "   MCP-NG Full Stack Installation"
echo "   Date: $(date)"
echo "=============================================="
echo

DOCKER_STATUS="NOT_ATTEMPTED"

echo "[1/4] Building Core Server & Tools..."
if ./setup/build.sh; then
    echo "✅ Server Build Successful"
else
    echo "❌ Server Build Failed"
    exit 1
fi

echo
echo "[2/4] Building Gatekeeper..."
if ./setup/build_gatekeeper.sh; then
    echo "✅ Gatekeeper Build Successful"
else
    echo "❌ Gatekeeper Build Failed"
    exit 1
fi

echo
echo "[3/4] Installing Admin UI Dependencies..."
if ./setup/build_admin.sh; then
    echo "✅ Admin UI Setup Successful"
else
    echo "❌ Admin UI Setup Failed"
    exit 1
fi

echo
echo "[4/4] Launching Observability Stack (Docker)..."
# Use a subshell or disable set -e temporarily for the docker command
set +e
docker compose up -d
DOCKER_EXIT_CODE=$?
set -e

if [ $DOCKER_EXIT_CODE -eq 0 ]; then
    echo "✅ Docker Stack Started"
    DOCKER_STATUS="SUCCESS"
else
    echo "⚠️  Docker Start Failed (Non-Critical)"
    DOCKER_STATUS="FAILED"
fi

echo
echo "=============================================="
echo "   Installation Complete!"
echo "=============================================="
echo
echo "Summary:"
echo " - Core Server:   INSTALLED"
echo " - Gatekeeper:    INSTALLED"
echo " - Admin UI:      INSTALLED"
if [ "$DOCKER_STATUS" == "SUCCESS" ]; then
    echo " - Docker Stack:  RUNNING"
else
    echo " - Docker Stack:  FAILED (See Warning)"
fi

echo
echo "----------------------------------------------"
echo "Usage Instructions:"
echo "1. Start Server:      ./run/run_server.sh"
echo "2. Start Gatekeeper:  ./run/run_gatekeeper.sh"
echo "3. Start Admin UI:    ./run/run_admin.sh"
echo "   -> Open Browser:   http://localhost:7860"
echo "----------------------------------------------"

if [ "$DOCKER_STATUS" == "FAILED" ]; then
    echo
    echo "⚠️  WARNING: Observability Stack (Loki/Alloy) did not start."
    echo "   Reason: Docker command failed."
    echo "   Action: 1. Ensure Docker Desktop/Daemon is running."
    echo "           2. Check network connection."
    echo "           3. Send '$LOG_FILE' to support."
    echo "   You can try starting it manually later: 'docker compose up -d'"
fi

echo
echo "Log saved to: $LOG_FILE"
