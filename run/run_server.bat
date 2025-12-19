@echo off
rem Change directory to the project root to ensure relative paths work.
cd /d "%~dp0..\"
echo Starting MCP-NG Meta-Server... 1>&2
.\\MCP-NG\\bin\\server.exe %*
