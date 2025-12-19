# FastAPI Calculator (Python)

## Description

The `fastapi_calculator` is a Python-based tool that provides mathematical evaluation capabilities. Despite the name (legacy), it is now a fully gRPC-compliant microservice that integrates with the Meta-Server.

It uses Python's `eval()` function to calculate results, making it flexible but potentially risky if exposed to untrusted input (though the execution environment is isolated).

## Parameters

The tool accepts the following arguments in a JSON object:

| Parameter    | Type     | Required | Description                             |
|--------------|----------|----------|-----------------------------------------|
| `expression` | `string` | **Yes**  | The mathematical expression to evaluate.|

## Usage Example

**Calculate an expression:**

```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "fastapi_calculator",
    "arguments": {
      "expression": "(10 + 5) * 3"
    }
  },
  "id": 1
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "45"
      }
    ]
  }
}
```

## Configuration

This tool runs as a Python process managed by Poetry.

**Configuration (`config.json`):**
```json
{
    "name": "fastapi_calculator",
    "description": "Evaluate python math expression. Backup calculator.",
    "command": ["poetry", "run", "python", "server.py"]
}
```

## Dependencies
*   `grpcio`
*   `protobuf`
*   `mcp-shared-proto-py`
