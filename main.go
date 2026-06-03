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
	"crypto/boring"
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
		Version: "0.3.1",
	}, nil)

	fmt.Fprintf(os.Stderr, "Diagram generation: %v\n", defaultEnableDiagram)

	thinkingServer := NewDeepThinkingServer(*workerCount, *maxWorkerCount, *shmRoot, defaultEnableDiagram)
	fmt.Fprintf(os.Stderr, "BoringCrypto (FIPS): %v\n", boring.Enabled())

	// Register the deepthinking tool (Hybrid Mode: Supports both Single and Batch/Turbo)
	mcp.AddTool(s, &mcp.Tool{
		Name: "deepthinking",
		Description: `A high-performance tool for dynamic and reflective problem-solving through thoughts with GPT (Gather, Process, Test) workflow.
This tool supports both single thinking steps and multiple thinking steps in a single call (Turbo Mode).

🚨 CRITICAL RULES & NUDGES:
1. MANDATORY FIELDS: Each thought MUST provide 'thought', 'thoughtNumber', 'totalThoughts', and 'nextThoughtNeeded'.
2. CAMELCASE ONLY: Use 'totalThoughts', NOT 'total_thoughts'. All parameters are camelCase.
3. GPT WORKFLOW:
   - G (Gather): Multiple workers (default 5) generate different solutions. You MUST use this phase to explore at least 5 different perspectives before moving to 'process'.
   - P (Process): A single worker processes the chosen solution.
   - T (Test): A single worker tests and verifies the result.
4. NEW THINKING ISOLATION: When beginning a brand new thinking task (starting with thoughtNumber=1), you MUST ALWAYS call the 'reset_thinking' tool first.

💡 TURBO MODE:
To use Turbo Mode, pass an array of thoughts in the 'thoughts' field. This is highly recommended for the 'gather' phase.

Parameters:
- thought: Your current thinking step (Single Mode).
- thoughts: An array of thinking steps (Turbo Mode).
- nextThoughtNeeded: True if you need more thinking.
- thoughtNumber: Current number in sequence.
- totalThoughts: Current estimate of thoughts needed.
- phase: "gather", "process", or "test".
- workerId: ID of the current worker.
- thinkingWorkerCount: Total number of workers for the Gather phase.
- track: The thinking track to use (e.g., 'bug-fix', 'feature', 'security').
- context: (Optional) Discovered tools or environment details.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"thought":             map[string]any{"type": "string"},
				"thoughtNumber":       map[string]any{"type": "integer"},
				"totalThoughts":       map[string]any{"type": "integer"},
				"nextThoughtNeeded":   map[string]any{"type": "boolean"},
				"phase":               map[string]any{"type": "string", "enum": []string{"gather", "process", "test"}},
				"workerId":            map[string]any{"type": "integer"},
				"thinkingWorkerCount": map[string]any{"type": "integer"},
				"track":               map[string]any{"type": "string"},
				"context":             map[string]any{"type": "string"},
				"isRevision":          map[string]any{"type": "boolean"},
				"revisesThought":      map[string]any{"type": "integer"},
				"branchFromThought":   map[string]any{"type": "integer"},
				"branchId":            map[string]any{"type": "string"},
				"generateDiagram":     map[string]any{"type": "boolean"},
				"isPrivate":           map[string]any{"type": "boolean"},
				"isTainted":           map[string]any{"type": "boolean"},
				"complexity":          map[string]any{"type": "string"},
				"thoughts": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"thought", "thoughtNumber", "totalThoughts", "nextThoughtNeeded"},
						"properties": map[string]any{
							"thought":             map[string]any{"type": "string"},
							"thoughtNumber":       map[string]any{"type": "integer"},
							"totalThoughts":       map[string]any{"type": "integer"},
							"nextThoughtNeeded":   map[string]any{"type": "boolean"},
							"phase":               map[string]any{"type": "string", "enum": []string{"gather", "process", "test"}},
							"workerId":            map[string]any{"type": "integer"},
							"thinkingWorkerCount": map[string]any{"type": "integer"},
							"track":               map[string]any{"type": "string"},
							"context":             map[string]any{"type": "string"},
							"isRevision":          map[string]any{"type": "boolean"},
							"revisesThought":      map[string]any{"type": "integer"},
							"branchFromThought":   map[string]any{"type": "integer"},
							"branchId":            map[string]any{"type": "string"},
							"generateDiagram":     map[string]any{"type": "boolean"},
							"isPrivate":           map[string]any{"type": "boolean"},
							"isTainted":           map[string]any{"type": "boolean"},
							"complexity":          map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (res *mcp.CallToolResult, resObj any, resErr error) {
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

			// Try to unmarshal as batch first
			var batch struct {
				Thoughts []ThoughtData `json:"thoughts"`
			}
			var thoughts []ThoughtData
			if err := json.Unmarshal(args, &batch); err == nil && len(batch.Thoughts) > 0 {
				thoughts = batch.Thoughts
			} else {
				// Fallback to single thought
				var single ThoughtData
				if err := json.Unmarshal(args, &single); err == nil && single.Thought != "" {
					thoughts = []ThoughtData{single}
				} else {
					ch <- callResult{
						res: &mcp.CallToolResult{
							IsError: true,
							Content: []mcp.Content{
								&mcp.TextContent{Text: "Invalid input: must provide either 'thought' or 'thoughts' array"},
							},
						},
						resObj: nil,
						resErr: nil,
					}
					return
				}
			}

			resp, err := thinkingServer.ProcessBatchThoughts(thoughts)
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
