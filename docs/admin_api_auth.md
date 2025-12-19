# Admin API Authentication

The MCP-NG Meta-Server exposes protected administrative endpoints for tool management:
*   `POST /admin/tools/scan`: Triggers a hot-reload of all tools in the `tools/` directories.
*   `POST /admin/tools/delete?name=<tool_name>`: Stops a running tool, removes its files, and unregisters it.

## Security Mechanism: The Gatekeeper

To secure these powerful endpoints, MCP-NG uses a dedicated proxy service called the **Admin Gatekeeper**.

### Architecture
1.  **The Gatekeeper (Proxy):** A standalone process that runs on a separate port (e.g., 8082). It handles authentication and forwards requests to the Meta-Server.
2.  **The Meta-Server:** Locks its administrative features (including visibility of meta-tools like `tool_manager`) until it is "claimed" by a Gatekeeper.

### How it Works
1.  **Startup:** You start the Meta-Server first.
2.  **Claiming:** You run the Gatekeeper using the provided scripts (`run/run_gatekeeper.sh`).
3.  **Token Generation:** The Gatekeeper automatically generates a secure, random `ADMIN_TOKEN`.
4.  **Handshake:** The Gatekeeper sends this token to the Meta-Server via `/admin/init`.
5.  **Lockdown:** The Meta-Server accepts the token, locks its admin API to require the `X-MCP-Admin-Key` header, and hot-reloads all tools (launching any pending tools that required the admin secret).

### Usage

**1. Start the Meta-Server**
```bash
./run/run_server.sh --transport=http --no-auth &
```

**2. Start the Gatekeeper**
```bash
./run/run_gatekeeper.sh
```
This will start the proxy on port `8082` (by default).

**3. Connect as Administrator**
Configure your Admin Agent (e.g., a separate LLM instance) to connect to the **Gatekeeper's URL**:
`http://127.0.0.1:8082`

Through this port, the agent will see all tools, including `tool_manager`, and requests will be automatically authenticated.

**4. Connect as User**
Standard users connect to the **Meta-Server's URL**:
`http://127.0.0.1:8080`

Through this port, users see standard tools but **cannot** see or use administration tools.

## Manual Configuration (Legacy / Fallback)

If you cannot use the Gatekeeper, you can manually create the token file before starting the server:

```bash
echo "my-super-secret-token-123" > env/ADMIN_TOKEN
```
*Note: This method does not provide the dynamic "Claiming" security benefits and requires manual tool restarts if the token changes.*
