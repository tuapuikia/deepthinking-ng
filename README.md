# DeepThinking-NG MCP

A high-performance Sequential Thinking MCP server with **GPT (Gather, Process, Test)** workflow, **Thinking Tracks**, and **Linux Shared Memory** support.

## Features

- **GPT Workflow**:
  - **Gather (G)**: Multiple workers (default 5) generate diverse solutions.
  - **Process (P)**: A single worker implements the chosen solution.
  - **Test (T)**: A single worker verifies the result.
- **Thinking Tracks**: Specialized modes that provide tailored guidance for different engineering tasks. You can use the built-in tracks or define your own **custom track** directly in the prompt:
  - `bug-fix`: Focuses on root cause isolation and regression prevention.
  - `feature`: Focuses on scalability and architectural alignment.
  - `security`: Focuses on threat modeling and defense-in-depth.
  - `custom-name`: (e.g., `refactor`, `performance`) Provides dynamic guidance for any alphanumeric track name (up to 32 characters).
- **LLM-Powered Synthesis**: Dynamically synthesizes multiple worker perspectives into a single, high-fidelity "Super Idea" using track-specific strategic prompts. Custom tracks receive generalized high-quality engineering guidance.
- **Context-Aware Thinking**: Allows injecting repository-specific context (e.g., tool discovery, environment info, or code patterns) into the thinking process via the `context` parameter, ensuring grounded and relevant reasoning.
- **Visual Thinking (Markdown Flowchart)**: Generates a markdown-native ASCII flowchart of the thinking process (phases, workers, branches, and revisions). This is an **opt-in** feature designed for troubleshooting and performance analysis without requiring external tools.
- **Shared Memory**: Uses `/dev/shm` to share thoughts between workers in the Gather phase, enabling the synthesis of a "Super Idea".
- **Dynamic Thinking**: Adjust total thoughts, branch, and revise as understanding deepens.
- **Interactive Logging**: Beautifully formatted console output with phase and worker identification.

## Security & Privacy

DeepThinking-NG is designed with a "Protect the Fort" philosophy to ensure your sensitive data remains secure.

### Deep Redaction
The server implements **Deep Redaction** at the entry point:
- **Input Filtering**: All thoughts are scanned for sensitive patterns (API keys, tokens, private keys) as soon as they are received.
- **No Leak to Memory/Disk**: Secrets are redacted *before* being stored in memory or written to shared memory files.
- **No Leak to LLM**: The server only sends redacted thoughts back to the LLM during the synthesis phase.
- **Supported Patterns**: GitHub tokens, OpenAI keys, AWS IDs, Google API keys, Slack tokens, and Private Keys (RSA/Generic).

### Shared Memory Security
- **Strict Path Enforcement**: The `SHM_ROOT` is strictly restricted to `/dev/shm` or its subdirectories. Any attempt to use paths outside of `/dev/shm` will result in a fallback to the default safe path.
- **Restrictive Permissions**: All directories and files created in shared memory use restrictive permissions (`0700` for directories, `0600` for files), ensuring only the user running the server can access them.
- **Session Isolation**: Each session is isolated into its own subdirectory using a random UUID to prevent cross-session data leakage.

### Secure Deployment Recommendation
For maximum security, it is highly recommended to run this MCP server in an **isolated environment**, such as a Docker container or a dedicated sandbox. While the server implements multiple layers of protection (Deep Redaction, restrictive permissions, and path enforcement), running in a shared environment still carries inherent risks. 

**User Responsibility**: Ensuring the "fort" is secure is a shared responsibility. While the code tries its best to avoid leakage, you should ensure the environment where the server runs is properly secured and isolated to prevent any potential data exposure.

## Installation

```bash
go build -o deepthinking-ng .
```

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

You can configure the server using environment variables:

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `THINKING_WORKER_COUNT` | Number of workers for the Gather phase. | `5` |
| `SHM_ROOT` | Root directory for shared memory storage. | `/dev/shm/deepthinking-ng` |
| `GEMINI_SESSION_ID` | Unique ID for the session to isolate shared memory. | `<Random UUID>` |
| `DISABLE_THOUGHT_LOGGING` | Set to `true` to disable console logging of thoughts. | `false` |

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
- `thought` (string): Your current thinking step.
- `phase` (string): "gather", "process", or "test" (default: "gather").
- `workerId` (int): ID of the current worker (1, 2, 3...).
- `thinkingWorkerCount` (int): Total workers for Gather phase (overrides env var).
- `thoughtNumber` (int): Current thought number.
- `totalThoughts` (int): Estimated total thoughts.
- `nextThoughtNeeded` (bool): Whether more thinking is required.
- `isRevision` (bool): Whether this revises a previous thought.
- `revisesThought` (int): The thought number being revised.
- `branchFromThought` (int): The thought number to branch from.
- `branchId` (string): Unique ID for the branch.
- `track` (string): The thinking track to use (e.g., `bug-fix`, `feature`, `security`, or a custom name like `refactor`).
- `context` (string): Repository-specific context or environmental information to ground the thinking.
- `generateDiagram` (bool): Set to `true` to generate a markdown-native ASCII flowchart in the response (opt-in).

### `reset_thinking`
Resets the thinking session and clears all shared memory in `/dev/shm`.

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
To generate a markdown-native ASCII flowchart for troubleshooting or analysis, set `generateDiagram` to `true` in any thinking step (usually the final one).

#### Natural Language Triggers
Since DeepThinking-NG is used by LLM agents, you can simply ask the agent to show the diagram using natural language:
- *"Show me the thinking flowchart."*
- *"Use deepthinking with diagram."*
- *"I want to see the thinking path."*

The agent will interpret your request and automatically set the `generateDiagram: true` flag in the next tool call.

- **Request**:
...

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
