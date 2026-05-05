# DeepThinking-NG MCP

A high-performance Sequential Thinking MCP server with **GPT (Gather, Process, Test)** workflow, **Thinking Tracks**, and **Linux Shared Memory** support.

## Features

- **GPT Workflow**:
  - **Gather (G)**: Multiple workers (default 5) generate diverse solutions.
  - **Process (P)**: A single worker implements the chosen solution.
  - **Test (T)**: A single worker verifies the result.
  - **Strict Enforcement**: The server enforces the logical flow (Gather -> Process -> Test) to ensure architectural integrity.
- **Thinking Tracks**: Specialized modes that provide tailored guidance for different engineering tasks. You can use the built-in tracks or define your own **custom track** directly in the prompt:
  - `bug-fix`: Focuses on root cause isolation and regression prevention.
  - `feature`: Focuses on scalability and architectural alignment.
  - `security`: Focuses on threat modeling and defense-in-depth.
  - `custom-name`: (e.g., `refactor`, `performance`) Provides dynamic guidance for any alphanumeric track name (up to 32 characters).
- **LLM-Powered Synthesis**: Dynamically synthesizes multiple worker perspectives into a single, high-fidelity "Super Idea" using track-specific strategic prompts. Custom tracks receive generalized high-quality engineering guidance.
- **Context-Aware Thinking**: Allows injecting repository-specific context (e.g., tool discovery, environment info, or code patterns) into the thinking process via the `context` parameter, ensuring grounded and relevant reasoning.
- **Visual Thinking (Markdown Flowchart)**: Generates a markdown-native ASCII flowchart of the thinking process. This is **enabled by default** and automatically saved to `deepthinking-flow.md` for persistent review.
- **Cross-Platform Shared Memory**: High-performance coordination between workers using volatile storage:
  - **Linux**: `/dev/shm` (RAM-based)
  - **macOS**: `/tmp` or `/private/tmp`
  - **Windows**: `%TEMP%`
- **Dynamic Scaling**: Suggests an optimal worker count based on task length (incremental scaling) and LLM-driven assessment of complexity. The server provides a structural suggestion that ramps up as the prompt length increases.
- **Interactive Logging**: Beautifully formatted console output with phase and worker identification.
- **Strong Nudges & Self-Correction**: The server implements a "Strong Nudge" system to help LLMs follow the rules:
  - **Mandatory Field Enforcement**: Explicitly marks `thought`, `thoughtNumber`, `totalThoughts`, and `nextThoughtNeeded` as required in the schema.
  - **Self-Correction Protocol**: The tool description includes a protocol for the LLM to immediately self-correct if a validation error occurs.
  - **Conversational Error Messages**: Replaces cryptic errors with helpful, "nudge-like" guidance (e.g., *"💡 NUDGE: You're trying to enter the 'process' phase, but the 'gather' phase is still incomplete..."*).

## Security & Privacy

DeepThinking-NG is designed with a "Protect the Fort" philosophy to ensure your sensitive data remains secure.

### Deep Redaction
The server implements **Deep Redaction** at the entry point:
- **Input Filtering**: All thoughts are scanned for sensitive patterns (API keys, tokens, private keys) as soon as they are received.
- **No Leak to Memory/Disk**: Secrets are redacted *before* being stored in memory or written to shared memory files.
- **No Leak to LLM**: The server only sends redacted thoughts back to the LLM during the synthesis phase.
- **Supported Patterns**: GitHub tokens, OpenAI keys, AWS IDs, Google API keys, Slack tokens, and Private Keys (RSA/Generic).

### Shared Memory Security
- **Strict Path Enforcement**: The `SHM_ROOT` is strictly restricted to volatile storage areas (e.g., `/dev/shm` on Linux, `/tmp` on macOS, or `%TEMP%` on Windows). Any attempt to use paths outside these areas will result in a fallback to the default safe path for the current OS.
- **Restrictive Permissions**: All directories and files created in shared memory use restrictive permissions (`0700` for directories, `0600` for files), ensuring only the user running the server can access them.
- **Session Isolation**: Each session is isolated into its own subdirectory using a random UUID to prevent cross-session data leakage.

### Secure Deployment Recommendation
For maximum security, it is highly recommended to run this MCP server in an **isolated environment**, such as a Docker container or a dedicated sandbox. While the server implements multiple layers of protection (Deep Redaction, restrictive permissions, and path enforcement), running in a shared environment still carries inherent risks. 

**User Responsibility**: Ensuring the "fort" is secure is a shared responsibility. While the code tries its best to avoid leakage, you should ensure the environment where the server runs is properly secured and isolated to prevent any potential data exposure.

## Installation

### Local Build
For a quick local build:
```bash
go build -o deepthinking-ng .
```

### Reproducible Builds
To ensure the binary is bit-for-bit identical and verifiable, use the Docker-based build system. This uses a fixed Go version (`1.26.2`) and a controlled environment to eliminate variations caused by local toolchains.

**Prerequisites**: Docker must be installed and running.

#### Build All Platforms
This command builds binaries for Linux, Windows, and macOS (amd64/arm64) inside a container:
```bash
make docker-reproducible-build
```
The resulting binaries and a `checksums.txt` file will be located in the `dist/` directory.

#### Verification & Integrity
You can verify the integrity of the build using SHA256 checksums.

1. **Generate Reference Checksum**:
   If you are a maintainer releasing a new version, generate the reference checksum:
   ```bash
   make checksum
   ```
   This creates `deepthinking-ng.sha256` based on the reproducible Linux binary.

2. **Verify Against Reference**:
   To confirm your local build matches the official reference:
   ```bash
   make verify
   ```
   This will perform a fresh reproducible build and compare its output against the `deepthinking-ng.sha256` file. If they match, you'll see: `Verification SUCCESS: Build matches reference!`.

## Running the Server

### Stdio Transport (Default)
```bash
./deepthinking-ng -transport stdio
```

### SSE Transport
```bash
./deepthinking-ng -transport sse -port 8080
```

### Gemini CLI Configuration

Add the following to your `gemini-cli` configuration (usually in `~/.gemini/config.json`):

```json
{
  "mcpServers": {
    "deepthinking-ng": {
      "command": "deepthinking-ng",
      "args": [],
      "env": {
        "THINKING_WORKER_COUNT": "5",
        "SHM_ROOT": "/dev/shm/deepthinking-ng"
      }
    }
  }
}
```

## Configuration Options

You can configure the server using environment variables or command-line flags:

| Environment Variable | Flag | Description | Default |
|----------------------|------|-------------|---------|
| `THINKING_WORKER_COUNT` | `-thinking-worker` | Number of workers for the Gather phase. | `5` |
| `MAX_THINKING_WORKER_COUNT` | `-max-thinking-worker` | Maximum allowed workers for the Gather phase. | `10` |
| `SHM_ROOT` | `-shm-root` | Root directory for shared memory storage. | OS-specific |
| `DISABLE_DIAGRAM` | `-disable-diagram` | Set to `true` to disable flowchart generation by default. | `false` |
| `GEMINI_SESSION_ID` | N/A | Unique ID for the session to isolate shared memory. | `<Random UUID>` |
| `DISABLE_THOUGHT_LOGGING` | N/A | Set to `true` to disable console logging of thoughts. | `false` |

### OS-Specific Defaults for `SHM_ROOT`
- **Linux**: `/dev/shm/deepthinking-ng`
- **macOS**: `/tmp/deepthinking-ng`
- **Windows**: `%TEMP%\deepthinking-ng`

## Session Isolation

To prevent collisions between different `gemini-cli` instances, the server automatically isolates shared memory into session-specific subdirectories under `SHM_ROOT`. 

- **Random UUID**: By default, it generates a random 128-bit UUID (hex encoded) as the session identifier when the server starts. This ID remains stable for the entire life of the server process and is returned in every tool response as `sessionId`.
- **Manual Override**: You can override this by setting the `GEMINI_SESSION_ID` environment variable if you need to maintain state across server restarts.

> [!IMPORTANT]
> **Non-Persistent Memory Warning**: The memory stored in `/dev/shm` is a RAM-based filesystem. It is **NOT persistent** and will be lost if the system reboots or if the shared memory is cleared. Do not use this for long-term storage; it is strictly for high-performance, short-term coordination between thinking workers.

## Tools

### `sequentialthinking`
The primary tool for the thinking process.

**Parameters:**
- `thought` (string): **REQUIRED**. Your current thinking step.
- `thoughtNumber` (int): **REQUIRED**. Current thought number.
- `totalThoughts` (int): **REQUIRED**. Estimated total thoughts.
- `nextThoughtNeeded` (bool): **REQUIRED**. Whether more thinking is required.
- `phase` (string): "gather", "process", or "test" (default: "gather").
- `workerId` (int): ID of the current worker (1, 2, 3...).
- `thinkingWorkerCount` (int): Total workers for Gather phase (overrides env var).
- `isRevision` (bool): Whether this revises a previous thought.
- `revisesThought` (int): The thought number being revised.
- `branchFromThought` (int): The thought number to branch from.
- `branchId` (string): Unique ID for the branch.
- `track` (string): The thinking track to use (e.g., `bug-fix`, `feature`, `security`, or a custom name like `refactor`).
- `context` (string): Repository-specific context or environmental information to ground the thinking.
- `generateDiagram` (bool): Set to `true` to generate a markdown-native ASCII flowchart in the response. **Enabled by default** unless disabled via server config.
- `isPrivate` (bool): **Zero-Knowledge**: If true, this thought will be redacted from the final synthesis and allWorkerThoughts.
- `isTainted` (bool): **Taint Analysis**: Mark this thought as untrusted (e.g., if it contains data from an unverified source).
- `complexity` (string): Optional metadata about the task complexity to help with dynamic scaling suggestions.

### `reset_thinking`
Resets the thinking session and clears all shared memory.

## GPT Workflow Example

### Using a Built-in Track (`bug-fix`)
1. **Gather Phase**:
   - Worker 1: `{"thought": "Solution A...", "phase": "gather", "workerId": 1, "track": "bug-fix"}`
   - ...
   - *Result*: Returns synthesized **Super Idea** with specialized bug-fixing guidance.

### Using a Custom Track (`performance`)
1. **Gather Phase**:
   - Worker 1: `{"thought": "Optimization A...", "phase": "gather", "workerId": 1, "track": "performance"}`
   - Worker 2: `{"thought": "Optimization B...", "phase": "gather", "workerId": 2, "track": "performance"}`
   - ...
   - *Result*: Returns synthesized **Super Idea** with dynamic guidance: *"As this is a 'PERFORMANCE' track, prioritize the core objectives of this mode..."*

2. **Process Phase**:
   - Worker 1: `{"thought": "Implementing synthesized strategy...", "phase": "process", "workerId": 1}`

3. **Test Phase**:
   - Worker 1: `{"thought": "Verifying implementation...", "phase": "test", "workerId": 1}`

### Visualizing the Thinking Process
DeepThinking-NG generates a markdown-native ASCII flowchart for troubleshooting or analysis. This is **enabled by default** and saved to `deepthinking-flow.md`.

#### Natural Language Triggers
Since DeepThinking-NG is used by LLM agents, you can simply ask the agent to show the diagram using natural language:
- *"Show me the thinking flowchart."*
- *"Use deepthinking with diagram."*
- *"I want to see the thinking path."*

The agent will interpret your request and ensure the `generateDiagram` flag is respected (or use the default).

- **Request**:
  ```json
  {
    "thought": "Finalizing and generating report...",
    "phase": "test",
    "generateDiagram": true
  }
  ```
- **Response**:
  Returns a `flowchart` field containing a markdown-native ASCII diagram:
  ```text
  🧠 DEEPTHINKING FLOWCHART
  =========================

  --- GATHER PHASE ---
  +--------------------+
  |      T1 (W1)       |
  +--------------------+
          |
          v
  +--------------------+
  |      T2 (W2)       |
  +--------------------+
  ```

---
*Developed with ❤️ for Dad.*
