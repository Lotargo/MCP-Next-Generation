# 🛡️ Reliability & Security Audit

This document summarizes the results of "Stress" and "Chaos" testing performed on the MCP-NG Meta-Server.

## 1. Security Stress Tests

### A. Auth Flood (DDoS Simulation)
*   **Test:** Flooded the server with requests containing invalid/missing Authorization headers.
*   **Result:** **Passed.**
    *   **Behavior:** The server instantly rejected invalid tokens with `401 Unauthorized`.
    *   **Protection:** Optional Basic Token Authentication (`--auth-token`) effectively blocks unauthorized access at the middleware level, preventing resource exhaustion.

### B. Rate Limiting / Fail2Ban (Advanced Flood Protection)
*   **Test:** Flooded server with invalid tokens from a single IP.
*   **Result:** **Passed.**
    *   **Behavior:** After 10 failed attempts, the server automatically banned the IP for 5 minutes.
    *   **Response:** `429 Too Many Requests`.
    *   **Efficiency:** Banned requests are rejected immediately without parsing headers or tokens, saving CPU.

### C. Large Payload (Buffer Overflow / OOM)
*   **Test:** Sent concurrent requests with **10MB** and **11MB** payloads.
*   **Result:** **Passed (Fixed).**
    *   **9MB Payload:** Accepted (`200 OK`).
    *   **11MB Payload:** Rejected (`413 Request Entity Too Large`).
    *   **Protection:** Implemented `http.MaxBytesReader` middleware with a 10MB hard limit. The server no longer attempts to read unbounded bodies into memory, preventing OOM attacks.

## 2. Resilience & Chaos Engineering

### D. Tool Process Crash (Chaos Monkey)
*   **Test:** While the system was under load (Calculator), we forcibly killed the tool process (`kill -9 <pid>`).
*   **Result:** **Passed (Resilient).**
    *   **Behavior:** The Meta-Server **survived** the death of its child processes.
    *   **Log:** `Tool process terminated unexpectedly tool=calculator`.
    *   **Recovery:** Subsequent requests to the killed tool return errors (as expected), but the server remains responsive to other requests.
    *   **Protection:** Implemented process monitoring goroutines in `launchTool` to catch exit signals and clean up resources without crashing the main process.

### E. Timeouts & Hangs
*   **Test:** Simulated tool "hangs" (30s+ latency) using `mock_web_search`.
*   **Result:** **Passed.**
    *   The server correctly waits for the tool. Client-side timeouts (HTTP Context cancellation) propagate to the server and cancel the gRPC call, releasing resources.

## 3. Summary

| Test Case | Status | Notes |
|-----------|--------|-------|
| **Auth Flood** | ✅ Passed | Auth token enabled |
| **Rate Limiter** | ✅ Passed | Fail2Ban active |
| **Large Payload** | ✅ Passed | 10MB Limit |
| **Tool Crash** | ✅ Passed | Server Survives |
| **Timeouts** | ✅ Passed | Standard Context |

*Tests performed using `tests/chaos_test.sh` and `tests/load_tester`.*

## 4. Security Model & Shared Responsibility

### Core vs. Tools
MCP-NG follows a strict separation of concerns:

*   **The Meta-Server (Core):** Designed to be secure, resilient, and high-performance. It protects itself against network attacks, resource exhaustion, and process failures.
*   **The Tools (Extensions):** The tools provided in this repository (e.g., `code_interpreter`, `file_writer`, `db_querier`) are **Reference Implementations**. They are designed to demonstrate functionality and capabilities.

### ⚠️ Important Disclaimer
**The tools run with the same privileges as the user launching the server.**
*   They are **NOT sandboxed** by default.
*   They perform operations (File I/O, Shell Execution) directly on the host machine.
*   Responsibility for tool safety (e.g., preventing path traversal, SQL injection, or malicious code execution) lies with the **Tool Developer** and the **System Administrator** deploying them.

**Recommendation:** For production environments involving sensitive data or untrusted input, we strongly recommend wrapping tools in containers (Docker) or VMs.
