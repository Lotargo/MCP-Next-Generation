# ⚡ Performance Benchmarks

This document details the performance characteristics of the MCP-NG Meta-Server under various load conditions. The goal is to demonstrate the system's ability to handle high concurrency with low latency, even on modest hardware.

## 🖥️ Test Environment

*   **Platform:** Google Cloud / Virtualized Environment
*   **Machine Type:** Comparable to `e2-medium` or similar standard instance.
*   **CPU:** Intel(R) Xeon(R) @ 2.30GHz (4 vCPUs)
*   **RAM:** 8 GB
*   **OS:** Linux (x86_64)

## 🧪 Methodology

*   **Tool:** Custom Go-based Load Tester (`tests/load_tester`).
*   **Target:** `calculator` tool (Go-based microservice).
*   **Operation:** Simple arithmetic (`10 + 20`).
*   **Protocol:** HTTP/1.1 (POST `tools/call` with persistent connections).
*   **Session:** Unique session per client thread.
*   **Sync Mode:** `X-Mcp-Sync: true` (forces synchronous execution for accurate latency timing).

## 📊 Results Summary

| Concurrent Clients | Total Requests | Throughput (RPS) | Avg Latency (ms) | P99 Latency (ms)* |
|--------------------|----------------|------------------|------------------|-------------------|
| **1**              | 100            | ~763             | 1.24 ms          | ~5.11 ms          |
| **10**             | 1,000          | ~2,937           | 3.24 ms          | ~8.52 ms          |
| **50**             | 5,000          | ~4,305           | 11.24 ms         | ~55.97 ms         |
| **100**            | 10,000         | ~7,149           | 13.54 ms         | ~63.26 ms         |

*\*P99 approximated from Max values in this test suite.*

## 🧪 Scenario B: Mixed Workload (Simulation)

To test the server's scheduler under realistic conditions, we ran a "Mixed" scenario where clients randomly select tasks:
*   **40% Calculator** (Fast CPU task)
*   **40% File I/O** (Fast Disk operations: Read/Write)
*   **20% Web Search** (Simulated Latency: 100ms - 800ms)

### Results (100 Concurrent Clients)
*   **Total Throughput:** ~976 RPS
*   **Stability:** 0 Failed Requests

**Latency Breakdown by Tool:**
| Tool Type | Avg Latency | Impact |
|-----------|-------------|--------|
| **Calculator** | **2.25 ms** | ✅ Unaffected by slow neighbors |
| **File I/O** | **~2.45 ms** | ✅ Unaffected by slow neighbors |
| **Web Search** | **453 ms** | ℹ️ Reflects simulated external API delay |

## 🧪 Scenario C: I/O Bound (Disk Stress)

This scenario tests the server's ability to handle rapid file system operations (50% Read, 50% Write).

| Concurrent Clients | Total Requests | Throughput (RPS) | Avg Latency (ms) |
|--------------------|----------------|------------------|------------------|
| **50**             | 2,000          | ~3,546           | 12.86 ms         |
| **100**            | 5,000          | ~4,271           | 22.39 ms         |

**Observation:** File system operations are highly optimized. The server efficiently handles resource requests, achieving >4000 ops/sec.

## 🧪 Scenario D: Connection Saturation (Network Stability)

This scenario forces clients to wait for a simulated **2000ms** external API delay, keeping connections open for extended periods. This tests the server's ability to manage high concurrency without running out of file descriptors or memory.

| Concurrent Clients | Total Requests | Server Overhead* | Failed Requests |
|--------------------|----------------|------------------|-----------------|
| **100**            | 1,000          | **4.6 ms**       | 0               |
| **200**            | 1,000          | **7.6 ms**       | 0               |

*\*Server Overhead = Avg Latency - Simulated Delay (2000ms). This represents the time spent inside the Meta-Server itself.*

**Observation:** Even with 200 simultaneous active connections waiting for "slow" tools, the Meta-Server adds less than **10ms** of overhead per request.

## 📈 Analysis

1.  **Non-Blocking Architecture:** The Mixed Workload test proves that long-running operations (like Web Search) **do not block** fast operations. While 20% of the threads were waiting for "external APIs" (sleeping for ~450ms), the server continued to serve Calculator and File requests in **~2ms**.
2.  **Scalable Concurrency:** Scenario D demonstrates that the server scales linearly. It handled 200 concurrent "slow" clients with zero failures and negligible overhead, proving it can support large teams or busy agents.
3.  **High Throughput:** The Go-based Meta-Server architecture demonstrates exceptional throughput, scaling to over **7,000 requests per second** (pure calc) on a 4-core machine.
2.  **Low Latency:** Under light load (1-10 users), the system adds negligible overhead (~1-3ms). Even under heavy load (100 concurrent requests), average latency remains under 15ms, providing a snappy experience for end-users.
3.  **Stability:** No errors were recorded during the stress tests (`Failed Requests: 0`). The server successfully managed connection queues without dropping requests.

## 📝 Conclusion

MCP-NG is production-ready for high-load scenarios. Its modular architecture does not introduce significant latency penalties compared to monolithic servers, while offering the benefits of isolation and stability.
