package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	transportType := flag.String("transport", "stdio", "Transport type (stdio or sse)")
	host := flag.String("host", "127.0.0.1", "Host for SSE transport")
	port := flag.Int("port", 8080, "Port for SSE transport")
	workerCount := flag.Int("thinking-worker", 0, "Number of workers for the Gather phase (overrides THINKING_WORKER_COUNT env)")
	maxWorkerCount := flag.Int("max-thinking-worker", 0, "Maximum number of workers for the Gather phase (overrides MAX_THINKING_WORKER_COUNT env)")
	shmRoot := flag.String("shm-root", "", "Root directory for shared memory storage (overrides SHM_ROOT env)")
	disableDiagram := flag.Bool("disable-diagram", false, "Disable markdown flowchart generation by default (overrides DISABLE_DIAGRAM env)")

	// Timeouts & Watchdog
	defaultHardTimeout := 60 * time.Second
	if envVal := os.Getenv("DEEPTHINKING_TIMEOUT"); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil {
			defaultHardTimeout = d
		}
	}
	hardTimeout := flag.Duration("timeout", defaultHardTimeout, "Hard timeout for tool execution (triggers self-termination watchdog) (overrides DEEPTHINKING_TIMEOUT env)")

	defaultSoftTimeout := 45 * time.Second
	if envVal := os.Getenv("DEEPTHINKING_SOFT_TIMEOUT"); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil {
			defaultSoftTimeout = d
		}
	}
	softTimeout := flag.Duration("soft-timeout", defaultSoftTimeout, "Soft timeout for tool execution (returns retryable error to client) (overrides DEEPTHINKING_SOFT_TIMEOUT env)")

	flag.Parse()

	defaultEnableDiagram := true
	if *disableDiagram || strings.ToLower(os.Getenv("DISABLE_DIAGRAM")) == "true" {
		defaultEnableDiagram = false
	}

	s := mcp.NewServer(&mcp.Implementation{
		Name:    "deepthinking-ng",
		Version: "0.3.0",
	}, nil)

	fmt.Fprintf(os.Stderr, "Diagram generation: %v\n", defaultEnableDiagram)

	thinkingServer := NewDeepThinkingServer(*workerCount, *maxWorkerCount, *shmRoot, defaultEnableDiagram)

	// Register the deepthinking tool
	mcp.AddTool(s, &mcp.Tool{
		Name: "deepthinking",
		Description: `A detailed tool for dynamic and reflective problem-solving through thoughts with GPT (Gather, Process, Test) workflow.
This tool helps analyze problems through a flexible thinking process that can adapt and evolve.

🚨 CRITICAL RULES & NUDGES:
1. MANDATORY FIELDS: You MUST always provide 'thought', 'thoughtNumber', 'totalThoughts', and 'nextThoughtNeeded'.
2. CAMELCASE ONLY: Use 'totalThoughts', NOT 'total_thoughts'. All parameters are camelCase.
3. GPT WORKFLOW:
   - G (Gather): Multiple workers (default 5) generate different solutions. You MUST use this phase to explore at least 5 different perspectives before moving to 'process'.
     * Use phase='gather' and increment 'workerId' (1, 2, 3, 4, 5).
     * AGENTIC DISCOVERY: In this phase, you should proactively use your available tools (e.g., filesystem tools, search tools, or other MCP tools) to gather context and ground your perspectives.
   - P (Process): A single worker processes the chosen solution.
     * Use phase='process'. Only available AFTER all gather steps are complete.
   - T (Test): A single worker tests and verifies the result.
     * Use phase='test'. Only available AFTER the process step is complete.
4. NEW THINKING ISOLATION: When beginning a brand new thinking task (starting with thoughtNumber=1), you MUST ALWAYS call the 'reset_thinking' tool first. This completely clears any shared memory (/dev/shm) and in-memory state from previous tasks to prevent state pollution.

💡 SELF-CORRECTION NUDGE:
If you receive a validation error like "params must have required property 'thought'", it means you missed one of the 4 mandatory fields. Immediately retry with all 4 fields included.

Shared Memory:
Uses /dev/shm to share thoughts between workers during the Gather phase to synthesize a "Super Idea".

DYNAMIC SCALING:
For complex problems (e.g., architectural design, complex debugging, advanced mathematics, or physics), you are encouraged to increase the 'thinkingWorkerCount' (up to 10) during the 'gather' phase to explore a wider range of perspectives. Use your own judgment to assess the difficulty of the task.

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
- needsMoreThoughts: Whether more thoughts are needed.
- track: The thinking track to use (e.g., 'bug-fix', 'feature', 'security').
- context: (Optional) Discovered tools, environment details, or filesystem context to ground the thinking.
- generateDiagram: (Optional) Set to true to generate a markdown-native ASCII flowchart. Default is controlled by server config.`,
		InputSchema: struct {
			Type       string         `json:"type"`
			Properties map[string]any `json:"properties,omitempty"`
			Required   []string       `json:"required,omitempty"`
		}{
			Type: "object",
			Properties: map[string]any{
				"thought": map[string]any{
					"type":        "string",
					"description": "Your current thinking step. (REQUIRED)",
				},
				"thoughtNumber": map[string]any{
					"type":        "integer",
					"description": "Current number in sequence. (REQUIRED)",
				},
				"totalThoughts": map[string]any{
					"type":        "integer",
					"description": "Current estimate of thoughts needed. (REQUIRED)",
				},
				"nextThoughtNeeded": map[string]any{
					"type":        "boolean",
					"description": "True if you need more thinking. (REQUIRED)",
				},
				"phase": map[string]any{
					"type":        "string",
					"description": "Phase of thinking (gather, process, test). Default: 'gather'",
					"enum":        []string{"gather", "process", "test"},
				},
				"workerId": map[string]any{
					"type":        "integer",
					"description": "ID of the current worker (1, 2, 3...). Default: 1",
				},
				"thinkingWorkerCount": map[string]any{
					"type":        "integer",
					"description": "Total number of workers for the Gather phase. Default: 5",
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
				},
				"context": map[string]any{
					"type":        "string",
					"description": "Discovered tools, environment details, or filesystem context to ground the thinking",
				},
				"generateDiagram": map[string]any{
					"type":        "boolean",
					"description": "Set to true to generate a markdown-native ASCII flowchart in the response. Default is controlled by server config (DISABLE_DIAGRAM).",
				},
				"isPrivate": map[string]any{
					"type":        "boolean",
					"description": "Zero-Knowledge: If true, this thought will be redacted from the final synthesis and allWorkerThoughts.",
				},
				"isTainted": map[string]any{
					"type":        "boolean",
					"description": "Taint Analysis: Mark this thought as untrusted (e.g., if it contains data from an unverified source).",
				},
				"complexity": map[string]any{
					"type":        "string",
					"description": "Optional metadata about the task complexity to help with dynamic scaling suggestions.",
				},
			},
			Required: []string{"thought", "thoughtNumber", "totalThoughts", "nextThoughtNeeded"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ThoughtData) (res *mcp.CallToolResult, resObj any, resErr error) {
		type callResult struct {
			res    *mcp.CallToolResult
			resObj any
			resErr error
		}
		ch := make(chan callResult, 1)
		startTime := time.Now()

		go func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "PANIC RECOVERED in background deepthinking tool: %v\n", r)
					ch <- callResult{
						res: &mcp.CallToolResult{
							IsError: true,
							Content: []mcp.Content{
								&mcp.TextContent{Text: fmt.Sprintf("Internal Server Error: Panic recovered: %v", r)},
							},
						},
						resObj: nil,
						resErr: nil,
					}
				}
			}()

			resp, err := thinkingServer.ProcessThought(args)
			if err != nil {
				ch <- callResult{
					res: &mcp.CallToolResult{
						IsError: true,
						Content: []mcp.Content{
							&mcp.TextContent{Text: err.Error()},
						},
					},
					resObj: nil,
					resErr: nil,
				}
				return
			}

			// Return the response as structured content
			jsonResp, _ := json.MarshalIndent(resp, "", "  ")
			ch <- callResult{
				res: &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: string(jsonResp)},
					},
				},
				resObj: resp,
				resErr: nil,
			}
		}()

		select {
		case res := <-ch:
			return res.res, res.resObj, res.resErr

		case <-ctx.Done():
			// Client cancelled/timed out.
			elapsed := time.Since(startTime)
			remaining := *hardTimeout - elapsed
			if remaining < 0 {
				remaining = 5 * time.Second
			}
			go func() {
				select {
				case <-ch:
					// finished normally
				case <-time.After(remaining):
					fmt.Fprintf(os.Stderr, "FATAL: DeepThinking execution hung after client cancellation! Watchdog triggered hard crash after %v to allow restart.\n", *hardTimeout)
					os.Exit(1)
				}
			}()

			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Request cancelled by client (context timeout)"},
				},
			}, nil, ctx.Err()

		case <-time.After(*softTimeout):
			// Soft timeout fired!
			remainingHardTimeout := *hardTimeout - *softTimeout
			if remainingHardTimeout < 0 {
				remainingHardTimeout = 5 * time.Second
			}

			go func() {
				select {
				case <-ch:
					fmt.Fprintln(os.Stderr, "INFO: Background thought process finished after soft timeout.")
				case <-time.After(remainingHardTimeout):
					fmt.Fprintf(os.Stderr, "FATAL: DeepThinking execution hung! Watchdog triggered hard crash after %v to allow restart.\n", *hardTimeout)
					os.Exit(1)
				}
			}()

			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("DeepThinking tool execution exceeded soft timeout of %v. The server remains running. You may retry your request.", *softTimeout)},
				},
			}, nil, nil
		}
	})

	// Register the reset_thinking tool
	mcp.AddTool(s, &mcp.Tool{
		Name:        "reset_thinking",
		Description: "Resets the thinking process and clears shared memory in /dev/shm.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (res *mcp.CallToolResult, resObj any, resErr error) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "PANIC RECOVERED in reset_thinking tool: %v\n", r)
				res = &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("Internal Server Error: Panic recovered: %v", r)},
					},
				}
				resObj = nil
				resErr = nil
			}
		}()

		thinkingServer.mu.Lock()
		defer thinkingServer.mu.Unlock()

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
		thinkingServer.thoughtHistory = make([]ThoughtData, 0)
		thinkingServer.branches = make(map[string][]ThoughtData)
		thinkingServer.currentFlowchartFile = ""

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Thinking process reset and shared memory cleared."},
			},
		}, nil, nil
	})

	ctx := context.Background()

	switch *transportType {
	case "stdio":
		fmt.Fprintln(os.Stderr, "DeepThinking MCP Server running on stdio")
		if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil {
			log.Fatalf("Stdio server failed: %v", err)
		}
	case "sse":
		handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return s
		}, nil)

		fmt.Fprintf(os.Stderr, "DeepThinking MCP Server running on SSE at %s:%d\n", *host, *port)
		if err := http.ListenAndServe(fmt.Sprintf("%s:%d", *host, *port), handler); err != nil {
			log.Fatalf("SSE server failed: %v", err)
		}
	default:
		log.Fatalf("Unknown transport type: %s", *transportType)
	}
}
