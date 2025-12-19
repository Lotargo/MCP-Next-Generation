# Integration Guide

This guide provides detailed instructions on how to integrate new and existing tools into the MCP-NG server. It also covers how to connect the MCP server with client applications and leverage ReAct patterns for intelligent tool use.

## Integrating a New Tool

Integrating a new tool involves creating a gRPC service and providing a simple configuration file. The main server is designed to automatically discover, launch, and manage any tool that follows this structure.

### 1. Define the Tool's gRPC Contract
All service definitions are centralized in `MCP-NG/proto/mcp.proto`. To add a new tool, you will typically implement the existing `Tool` service interface, which includes `GetDescription` and `Run`. After updating the `.proto` file, regenerate the gRPC code by running `./setup/generate_protos.sh` (Linux/macOS) or `setup\generate_protos.bat` (Windows).

### 2. Implement the Tool's gRPC Server
Implement the gRPC server for your new tool in Go or Python, following the structure of the existing tools in `MCP-NG/tools/`. Your implementation must define the logic for the `GetDescription` and `Run` methods.

### 3. Implement the Health Check Service
Your tool **must** implement the standard gRPC Health Checking Protocol. This allows the main MCP server to monitor its status and route traffic only to healthy instances.

### 4. Create the Configuration File
In your tool's directory, create a `config.json` file. This tells the Meta-Server how to run your tool.

**Example for a Go tool (`calculator/config.json`):**
```json
{
  "command": ["calculator"],
  "required_secrets": []
}
```
- `command`: The name of the executable (for Go) or the entrypoint script (for Python).
- `required_secrets`: An optional list of environment variable names (e.g. `["API_KEY"]`) that the tool needs. The server handles loading these securely.

## Integrating with a Client Application

You can connect to the MCP-NG server using two primary transport mechanisms: **HTTP** and **Stdio**.

### 1. HTTP Transport (Recommended for Web/Networked Clients)

This is the standard mode for network-based communication.

**Start the Server:**
Run the server and specify an HTTP port.
```bash
./MCP-NG/bin/server --transport=http --http-port=8080
```
The server will now be accessible at `http://localhost:8080`.

**Communication Flow:**
1.  **Handshake:** The client must first perform the MCP handshake by sending an `initialize` request to the `/` (root) endpoint. The server will respond with a unique `Mcp-Session-Id` header, which must be included in all subsequent requests.
2.  **Notification:** The client completes the handshake by sending a `notifications/initialized` notification.
3.  **Tool Calls:** Send JSON-RPC requests to the `/` (root) endpoint via HTTP POST.

### 2. Stdio Transport (Recommended for Local/Embedded Clients)

This mode is ideal for tightly integrated applications where the server is managed as a child process.

**Start the Server:**
Run the server with the `--transport=stdio` flag.
```bash
./MCP-NG/bin/server --transport=stdio
```
The server will now listen for requests on its standard input and write responses to its standard output.

**JSON-RPC Framing:**
The server's "smart" `stdio` adapter supports two framing protocols, which it auto-detects:
1.  **Newline-Delimited JSON (Recommended):** Each JSON-RPC message is sent on its own line, terminated by a newline character (`\n`). This is the simpler and preferred method. The new Go client at `MCP-NG/cmd/mcp-client/main.go` provides a reference implementation.
2.  **Content-Length Framing:** Messages can also be framed with a `Content-Length` header, followed by a blank line, and then the JSON payload. This is supported for legacy compatibility.

## Observability & Tracing

MCP-NG supports distributed tracing to correlate requests across components.

### Header: `X-Trace-Id`
-   **Format:** 32-character hexadecimal string (UUID-like).
-   **Behavior:**
    -   **Gatekeeper:** Generates a new ID if the header is missing. Logs it with the proxy request.
    -   **Meta-Server:** Extracts the ID, adds it to the request context, and includes it in all structured logs (`trace_id` field).

If you are integrating a custom client (e.g., a web UI or another agent), you should generate a unique ID for each request and send it in the `X-Trace-Id` header. This allows you to trace your request through the entire MCP-NG stack.

## Using ReAct Patterns for Tool Selection

The ReAct (Reason and Act) pattern allows a large language model (LLM) to reason about which tool to use for a given task, creating a loop of thought, action, and observation.

**User Prompt:** "What is the result of 15 times 3, and who is the current president of France?"

**ReAct Loop Example:**
1.  **Thought:** The user has two questions. I should solve the math problem first. I will look for a calculator tool.
    *   **Action:** Send a `tools/list` request to the MCP Server.
2.  **Observation:** The server returns a list including a "calculator" tool.
3.  **Thought:** Great, I will use the "calculator" tool with the expression "15 * 3".
    *   **Action:** Send a `tools/call` request with `name="calculator"` and `arguments={"expression": "15 * 3"}`.
4.  **Observation:** The tool returns a result of `{"result": 45}`.
5.  **Thought:** The first part is done. Now I need to find the president of France. The "web_search_tavily" tool seems appropriate.
    *   **Action:** Send a `tools/call` request with `name="web_search_tavily"` and `arguments={"query": "current president of France"}`.
6.  **Observation:** The tool returns a text snippet: "Emmanuel Macron is the current president of France."
7.  **Thought:** I have both answers. I can now form the final response.
    *   **Final Answer to User:** The result of 15 times 3 is 45. The current president of France is Emmanuel Macron.

By following this guide, you can extend the MCP server with new capabilities and integrate it into a variety of applications, building powerful and flexible intelligent agents.

## Client Configuration Examples

### LM Studio

LM Studio allows you to connect to MCP servers via a configuration file (typically located at `~/.cache/lm-studio/mcp-server-config.json` or configured via the UI).

**Dual-Mode Configuration (Example):**
This configuration connects to two instances simultaneously: one running locally via Stdio (ideal for file access) and one running via HTTP (remote/networked).

```json
{
  "mcpServers": {
    "mcp-ng-stdio": {
      "command": "C:\\path\\to\\MCP-NG\\bin\\server.exe",
      "args": [
        "--transport=stdio"
      ]
    },
    "mcp-ng-http": {
      "url": "http://127.0.0.1:8080"
    }
  }
}
```

**Note:** Ensure the path to the executable is absolute and appropriate for your OS (e.g., `/path/to/server` on Linux/macOS or `C:\\path\\to\\server.exe` on Windows).
