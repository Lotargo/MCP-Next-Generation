@echo off
cd /d "%~dp0..\MCP-NG\admin_ui"
echo Starting Admin UI on http://0.0.0.0:7860
poetry run python admin.py
