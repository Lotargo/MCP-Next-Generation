# End-to-End Testing Guide (MCP Spec: 2025-06-18)

This document provides instructions for performing end-to-end (E2E) tests on the MCP-NG system. These tests verify the full protocol handshake, authorization, and core functionalities like tool execution and resource access.

## Prerequisites

1.  **Go:** Ensure you have Go 1.24+ installed.
2.  **Python:** Ensure you have Python 3.8+ installed.
3.  **Python Dependencies:** Install Poetry and the project's dependencies:
    ```bash
    pip install poetry
    poetry install
    ```
4.  **Build Binaries:** All Go components (server, tools) must be compiled.
    ```bash
    ./setup/build.sh  # Linux/macOS
    setup\build_windows.bat # Windows
    ```

---

## E2E Test Scenarios

### Cleanup Before You Start

Ensure no old server or tool processes are running.
```bash
pkill -f "server" || true
```

### Scenario 1: Meta-Server Direct Connection (HTTP)

This test verifies the full protocol flow against the standalone Meta-Server using the HTTP transport.

- **Run Server:** `./MCP-NG/bin/server --http-port=8080 --grpc-port=50050 &`
- **Run Test (Go Client):**
    ```bash
    # Build the client first
    go build -o ./MCP-NG/bin/mcp-client ./MCP-NG/cmd/mcp-client
    # Run the client
    ./MCP-NG/bin/mcp-client
    ```

