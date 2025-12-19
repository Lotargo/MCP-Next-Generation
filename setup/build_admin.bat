@echo off
echo Installing Admin UI dependencies...
cd /d "%~dp0..\MCP-NG\admin_ui"
poetry install
if %errorlevel% neq 0 exit /b %errorlevel%
