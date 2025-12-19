# Visual Studio Code Setup for MCP-NG

If you are seeing errors like `Import "google.protobuf" could not be resolved from source` in Pylance, it means VS Code is not using the correct Python environment for the module you are editing.

This project is a monorepo with multiple Python modules managed by Poetry. Each module has its own virtual environment.

## Solution

1. **Install Dependencies**
   Ensure you have installed the dependencies for the module. From the module directory (e.g., `mcp-shared-proto-py`):
   ```bash
   poetry install
   ```

2. **Select Interpreter in VS Code**
   - Open the file where you see errors (e.g., `mcp_pb2.py`).
   - Open the Command Palette (`Ctrl+Shift+P` or `Cmd+Shift+P`).
   - Run **"Python: Select Interpreter"**.
   - Look for the virtual environment created by Poetry for this specific module. It will typically be named like `mcp-shared-proto-py-XXXXXX-py3.x`.
   - If it doesn't appear, you can find the path by running `poetry env info --path` in the terminal within that directory, and then pasting that path into the "Enter interpreter path..." option.

## Workspace Configuration (Optional)

If you work with the multi-root workspace, you can add a `.vscode/settings.json` to help Pylance find imports, but selecting the correct interpreter per-file or per-workspace root is the most reliable method.
