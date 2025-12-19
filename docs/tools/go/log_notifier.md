# Log Notifier Tool

## Description

The `log_notifier` tool is a simple Go-based gRPC service that writes a structured log message to its own standard output. It is primarily used for creating observable events from an agent's workflow, such as reporting task completion, marking important state changes, or flagging warnings.

The tool uses the standard `slog` library, producing JSON-formatted logs.

## Parameters

The tool accepts the following arguments in a JSON object:

| Parameter | Type     | Required | Default | Description                                                              |
|-----------|----------|----------|---------|--------------------------------------------------------------------------|
| `message` | `string` | **Yes**  |         | The notification message to be logged.                                   |
| `level`   | `string` | No       | `INFO`  | The importance level of the log. Supported values are `INFO`, `WARNING` (or `WARN`), and `ERROR`. Any other value will default to `INFO`. |

## Response

*   **Successful Log:** If the log is successfully written, the tool returns the string `"Notification successfully logged."`.
*   **Error:** If the required `message` parameter is missing, the tool returns an error message.

## Usage Example

Here is an example of how to use the `log_notifier` tool to log a warning message.

**Request:**

```bash
curl -X POST http://localhost:8080/ \
-H "Content-Type: application/json" \
-d '{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "log_notifier",
    "arguments": {
      "message": "The API returned an unexpected status code. Proceeding with default data.",
      "level": "WARNING"
    }
  },
  "id": 1
}'
```

**Successful Response:**

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Notification successfully logged."
      }
    ]
  }
}
```

This would cause the `log_notifier` tool's own process to write a JSON log to its standard output similar to this:
```json
{"time":"2023-10-27T10:30:00.123Z","level":"WARN","msg":"The API returned an unexpected status code. Proceeding with default data."}
```

## Configuration

The tool's configuration is managed through a `config.json` file located in the tool's directory.

**Example `config.json`:**
```json
{
  "command": ["log_notifier"]
}
```

## Health and Logging

*   **Health Checks:** This tool implements the standard gRPC Health Checking Protocol.
*   **Logging:** The tool's primary function is to create structured logs to its own standard output.