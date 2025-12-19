# Developing Tools for MCP-NG

MCP-NG is designed as a **polyglot** platform. Because the core architecture relies on **gRPC**, tools can be written in any language that supports gRPC. However, the level of "native" support (ease of integration, build scripts, shared libraries) varies by language.

## Supported Languages

| Language | Status | Build Support | Shared Libs | Notes |
| :--- | :--- | :--- | :--- | :--- |
| **Go** | 🟢 **Native** | Yes (Auto) | `toolcore` | Primary language. Fastest, mostly static. |
| **Python** | 🟢 **Native** | Yes (Poetry) | `mcp-shared-proto-py` | Ideal for AI/ML/Data Science. |
| **Rust** | 🟡 **Planned** | Manual | None | Requires manual gRPC setup. |
| **Node.js** | 🟡 **Planned** | Manual | None | Requires manual gRPC setup. |

---

## 1. Creating a Go Tool (Recommended)

Go tools are compiled into single binaries and managed directly by the `go.work` workspace.

### Structure
Create a directory: `MCP-NG/tools/go/<tool_name>`

### Prerequisites
*   `main.go`
*   `config.json`
*   `go.mod`

### Step-by-Step
1.  **Initialize Module:**
    ```bash
    go mod init <tool_name>
    ```
2.  **Configure `go.mod`:**
    You must replace the shared libraries with local versions.
    ```go
    require (
        mcp/shared/goutils v0.0.0
        mcp/shared/proto v0.0.0
        google.golang.org/protobuf v1.36.10
    )
    replace mcp/shared/goutils => ../../../../mcp-shared-goutils
    replace mcp/shared/proto => ../../../../mcp-shared-proto
    ```
3.  **Implement `main.go`:**
    Your tool must implement the `toolcore.Tool` interface.
    ```go
    package main
    import (
        "context"
        "flag"
        "mcp/shared/goutils/toolcore"
        pb "mcp/shared/proto"
    )
    type MyTool struct { pb.UnimplementedToolServer }
    // Implement GetDescription, Run, etc.
    func main() {
        port := flag.Int("port", 0, "Port")
        transport := flag.String("transport", "grpc", "Transport")
        flag.Parse()
        toolcore.Start(&MyTool{}, *transport, *port)
    }
    ```
4.  **Build:** Run `./setup/build.sh`.

---

## 2. Creating a Python Tool

Python tools run in their own virtual environments managed by Poetry.

### Structure
Create a directory: `MCP-NG/tools/python/<tool_name>`

### Prerequisites
*   `server.py`
*   `config.json`
*   `pyproject.toml`

### Step-by-Step
1.  **Initialize Poetry:**
    ```bash
    poetry init
    ```
2.  **Add Dependencies:**
    ```bash
    poetry add grpcio protobuf
    poetry add --editable ../../../../mcp-shared-proto-py
    ```
3.  **Implement `server.py`:**
    Implement the `mcp_pb2_grpc.ToolServicer` class.
    ```python
    import grpc
    from mcp_shared_proto_py import mcp_pb2, mcp_pb2_grpc
    class MyTool(mcp_pb2_grpc.ToolServicer):
        def GetDescription(self, request, context): ...
        def Run(self, request, context): ...
    # Start gRPC server on --port
    ```
4.  **Configure `config.json`:**
    ```json
    {
        "name": "my_tool",
        "command": ["poetry", "run", "python", "server.py"]
    }
    ```

---

## 3. Creating Tools in Other Languages (Rust, Node.js, etc.)

**Status:** 🟡 **Experimental / Manual**

To add a tool in a language without a helper SDK (like Rust), you must:

1.  **Implement the Protocol:**
    *   Use the `.proto` definitions from `MCP-NG/proto/mcp.proto`.
    *   Generate gRPC code for your language.
    *   Implement the `Tool` service.
2.  **Handle Flags:**
    *   Your executable must accept a `--port=<int>` flag.
    *   It must start a gRPC server on that port.
3.  **Configuration:**
    *   Create a `config.json` in your tool's folder.
    *   `command`: `["./path/to/your/executable"]` (or runtime command).
4.  **Build Integration:**
    *   The current `build.sh` **will not** automatically compile Rust or Node.js projects. You must compile them manually into `MCP-NG/bin/` or update the build scripts.

> **Future Roadmap:** We plan to add `toolcore` equivalents for Rust and Node.js to automate this process.

---

## 4. Modifying the Protocol

If you need to modify the core MCP protocol (e.g., adding a new method to `MCP-NG/proto/mcp.proto`), you must regenerate the shared libraries for both Go and Python.

### Regenerating Go Code
Run the following script to update `mcp-shared-proto`:
*   **Linux/macOS:** `./setup/generate_protos.sh`
*   **Windows:** `setup\generate_protos.bat`

### Regenerating Python Code
Run the following script to update `mcp-shared-proto-py`:
*   **Linux/macOS:** `./setup/generate_protos_python.sh`
*   **Windows:** `setup\generate_protos_python.bat`
