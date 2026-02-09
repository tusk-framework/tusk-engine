# 🐘 TUSK Native Engine Specification (v0.1)

## Overview
The **Tusk Native Engine** is the application server backbone of the Tusk Framework. It serves as the **Master Process** that manages the PHP lifecycle, providing a robust, long-running runtime environment.

## Implemented Architecture (v0.1)

### 1. Process Supervision (Go)
- **Pool Manager**: Spawns a configured number of `worker.php` processes.
- **Self-Healing**: Automatically restarts PHP workers if they crash.
- **Graceful Shutdown**: Handles SIGTERM/SIGINT to clean up workers.

### 2. Networking (Go)
- **HTTP/1.1**: Uses Go's native `net/http` server.
- **Dynamic Config**: Loads `tusk.json` to configure listener address and ports.

### 3. Inter-Process Communication (IPC)
- **Standard I/O Pipes**: uses `stdin` and `stdout` to communicate with workers.
- **Protocol**: NDJSON (Newline Delimited JSON).
    - **Request**: JSON payload containing Method, URL, Headers, and Body.
    - **Response**: JSON payload containing Status, Headers, and Body.

### 4. Zero-Dependency CLI
- **Unified Binary**: `tusk` binary acts as the entry point.
- **Proxy Mode**: Dispatches unknown commands to the PHP script (e.g., `tusk migrate` -> `php tusk migrate`).

## Future Roadmap
- **Protocol Upgrade**: Move to Protobuf or MsgPack for higher performance.
- **Shared Memory**: Implement `shm` for faster data exchange.
- **Metrics**: Native Prometheus exporter.
- **Hot Reload**: Watcher implementation for development mode.
