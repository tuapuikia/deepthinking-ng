# DeepThinking-NG MCP

A high-performance Sequential Thinking MCP server with **GPT (Gather, Process, Test)** workflow and **Linux Shared Memory** support.

## Features

- **GPT Workflow**:
  - **Gather (G)**: Multiple workers (default 5) generate diverse solutions.
  - **Process (P)**: A single worker implements the chosen solution.
  - **Test (T)**: A single worker verifies the result.
- **Shared Memory**: Uses `/dev/shm` to share thoughts between workers in the Gather phase, enabling the synthesis of a "Super Idea".
- **Dynamic Thinking**: Adjust total thoughts, branch, and revise as understanding deepens.
- **Interactive Logging**: Beautifully formatted console output with phase and worker identification.

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

## Configuration Options

You can configure the server using environment variables:

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `THINKING_WORKER_COUNT` | Number of workers for the Gather phase. | `5` |
| `SHM_ROOT` | Root directory for shared memory storage. | `/dev/shm/deepthinking-ng` |
| `DISABLE_THOUGHT_LOGGING` | Set to `true` to disable console logging of thoughts. | `false` |

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

### `reset_thinking`
Resets the thinking session and clears all shared memory in `/dev/shm`.

## GPT Workflow Example

1. **Gather Phase**:
   - Worker 1: `{"thought": "Solution A...", "phase": "gather", "workerId": 1}`
   - Worker 2: `{"thought": "Solution B...", "phase": "gather", "workerId": 2}`
   - Worker 3: `{"thought": "Solution C...", "phase": "gather", "workerId": 3}`
   - Worker 4: `{"thought": "Solution D...", "phase": "gather", "workerId": 4}`
   - Worker 5: `{"thought": "Solution E...", "phase": "gather", "workerId": 5}`
   - *Result*: The tool returns all 5 solutions and a synthesized **Super Idea**.

2. **Process Phase**:
   - Worker 1: `{"thought": "Implementing Super Idea...", "phase": "process", "workerId": 1}`

3. **Test Phase**:
   - Worker 1: `{"thought": "Verifying implementation...", "phase": "test", "workerId": 1}`

---
*Developed with ❤️ for Dad.*
