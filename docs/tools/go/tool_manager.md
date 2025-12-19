# Tool Manager

## Description

The `tool_manager` is a specialized administrative tool written in Go. Its primary purpose is to allow dynamic management of the MCP server's toolset at runtime, specifically enabling the safe deletion and unloading of other tools.

It connects to the Meta-Server's Admin API to perform these operations.

## Parameters

The tool accepts the following arguments in a JSON object:

| Parameter   | Type     | Required | Description                                    |
|-------------|----------|----------|------------------------------------------------|
| `operation` | `string` | **Yes**  | The operation to perform. Supported: `delete`, `list`. |
| `tool_name` | `string` | **Yes**  | The name of the tool to delete (required for `delete`). |

**Note:** The `list` operation in this tool simply points the user to the standard `tools/list` MCP method.

## Usage Example

**Delete a tool:**

```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "tool_manager",
    "arguments": {
      "operation": "delete",
      "tool_name": "unused_tool"
    }
  },
  "id": 1
}
```

## Security & Configuration

This tool is highly sensitive as it modifies the running state of the server. It requires the `ADMIN_TOKEN` secret to function.

**Configuration (`config.json`):**
```json
{
    "name": "tool_manager",
    "description": "Manages MCP tools: delete tools securely.",
    "command": ["tool_manager"],
    "required_secrets": ["ADMIN_TOKEN"]
}
```

The server automatically injects the `ADMIN_TOKEN` via a temporary file path passed to the `--admin_token-from-file` flag during startup.

## Limitations

*   Cannot delete system-critical tools: `tool_manager` itself, `file_writer`, `file_reader`.
