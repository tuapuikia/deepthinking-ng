package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	transportType := flag.String("transport", "stdio", "Transport type (stdio or sse)")
	host := flag.String("host", "127.0.0.1", "Host for SSE transport")
	port := flag.Int("port", 8080, "Port for SSE transport")
	workerCount := flag.Int("thinking-worker", 0, "Number of workers for the Gather phase (overrides THINKING_WORKER_COUNT env)")
	shmRoot := flag.String("shm-root", "", "Root directory for shared memory storage (overrides SHM_ROOT env)")
	flag.Parse()

	s := mcp.NewServer(&mcp.Implementation{
		Name:    "deepthinking-ng",
		Version: "0.2.0",
	}, nil)

	thinkingServer := NewSequentialThinkingServer(*workerCount, *shmRoot)

	// Register the sequentialthinking tool
	mcp.AddTool(s, &mcp.Tool{
		Name: "sequentialthinking",
		Description: `A detailed tool for dynamic and reflective problem-solving through thoughts with GPT (Gather, Process, Test) workflow.
This tool helps analyze problems through a flexible thinking process that can adapt and evolve.

GPT Workflow:
- G (Gather): Multiple workers (default 5) generate different solutions. You MUST use this phase to explore at least 5 different perspectives before moving to 'process'.
- P (Process): A single worker processes the chosen solution.
- T (Test): A single worker tests and verifies the result.

Shared Memory:
Uses /dev/shm to share thoughts between workers during the Gather phase to synthesize a "Super Idea".

Parameters:
- thought: Your current thinking step.
- nextThoughtNeeded: True if you need more thinking.
- thoughtNumber: Current number in sequence.
- totalThoughts: Current estimate of thoughts needed.
- phase: "gather", "process", or "test" (default: "gather").
- workerId: ID of the current worker (1, 2, 3...).
- thinkingWorkerCount: Total number of workers for the Gather phase (default: 5).
- isRevision: Boolean indicating if this revises previous thinking.
- revisesThought: Which thought number is being reconsidered.
- branchFromThought: Which thought number is the branching point.
- branchId: Identifier for the current branch.
- needsMoreThoughts: If reaching end but realizing more thoughts needed.`,
		InputSchema: struct {
			Type       string         `json:"type"`
			Properties map[string]any `json:"properties,omitempty"`
			Required   []string       `json:"required,omitempty"`
		}{
			Type: "object",
			Properties: map[string]any{
				"thought": map[string]any{
					"type":        "string",
					"description": "Your current thinking step",
				},
				"thoughtNumber": map[string]any{
					"type":        "integer",
					"description": "Current number in sequence",
				},
				"totalThoughts": map[string]any{
					"type":        "integer",
					"description": "Current estimate of thoughts needed",
				},
				"nextThoughtNeeded": map[string]any{
					"type":        "boolean",
					"description": "True if you need more thinking",
				},
				"phase": map[string]any{
					"type":        "string",
					"description": "Phase of thinking (gather, process, test)",
					"enum":        []string{"gather", "process", "test"},
				},
				"workerId": map[string]any{
					"type":        "integer",
					"description": "ID of the current worker",
				},
				"thinkingWorkerCount": map[string]any{
					"type":        "integer",
					"description": "Total number of workers for the Gather phase",
				},
				"isRevision": map[string]any{
					"type":        "boolean",
					"description": "Whether this is a revision of a previous thought",
				},
				"revisesThought": map[string]any{
					"type":        "integer",
					"description": "The thought number being revised",
				},
				"branchFromThought": map[string]any{
					"type":        "integer",
					"description": "The thought number to branch from",
				},
				"branchId": map[string]any{
					"type":        "string",
					"description": "Identifier for the current branch",
				},
				"needsMoreThoughts": map[string]any{
					"type":        "boolean",
					"description": "Whether more thoughts are needed",
				},
				"track": map[string]any{
					"type":        "string",
					"description": "The thinking track to use (e.g., 'bug-fix', 'feature', 'security')",
					"enum":        []string{"bug-fix", "feature", "security"},
				},
			},
			Required: []string{"thought", "thoughtNumber", "totalThoughts", "nextThoughtNeeded"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ThoughtData) (*mcp.CallToolResult, any, error) {
		resp, err := thinkingServer.ProcessThought(args)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil, nil
		}

		// Return the response as structured content
		jsonResp, _ := json.MarshalIndent(resp, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(jsonResp)},
			},
		}, resp, nil
	})

	// Register the reset_thinking tool
	mcp.AddTool(s, &mcp.Tool{
		Name:        "reset_thinking",
		Description: "Resets the thinking process and clears shared memory in /dev/shm.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		err := thinkingServer.shm.ClearAll()
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Failed to clear shared memory: %v", err)},
				},
			}, nil, nil
		}

		// Also reset in-memory state
		thinkingServer.mu.Lock()
		thinkingServer.thoughtHistory = make([]ThoughtData, 0)
		thinkingServer.branches = make(map[string][]ThoughtData)
		thinkingServer.mu.Unlock()

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Thinking process reset and shared memory cleared."},
			},
		}, nil, nil
	})

	ctx := context.Background()

	switch *transportType {
	case "stdio":
		fmt.Fprintln(os.Stderr, "Sequential Thinking MCP Server running on stdio")
		if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil {
			log.Fatalf("Stdio server failed: %v", err)
		}
	case "sse":
		handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return s
		}, nil)

		fmt.Fprintf(os.Stderr, "Sequential Thinking MCP Server running on SSE at %s:%d\n", *host, *port)
		if err := http.ListenAndServe(fmt.Sprintf("%s:%d", *host, *port), handler); err != nil {
			log.Fatalf("SSE server failed: %v", err)
		}
	default:
		log.Fatalf("Unknown transport type: %s", *transportType)
	}
}
