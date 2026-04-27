package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/fatih/color"
)

// ThoughtData represents the input for a single thinking step.
type ThoughtData struct {
	Thought             string  `json:"thought"`
	ThoughtNumber       int     `json:"thoughtNumber"`
	TotalThoughts       int     `json:"totalThoughts"`
	NextThoughtNeeded   bool    `json:"nextThoughtNeeded"`
	IsRevision          *bool   `json:"isRevision,omitempty"`
	RevisesThought      *int    `json:"revisesThought,omitempty"`
	BranchFromThought   *int    `json:"branchFromThought,omitempty"`
	BranchID            *string `json:"branchId,omitempty"`
	NeedsMoreThoughts   *bool   `json:"needsMoreThoughts,omitempty"`
	
	// GPT Workflow fields
	Phase               string  `json:"phase,omitempty"`               // "gather", "process", "test"
	WorkerID            int     `json:"workerId,omitempty"`            // 1, 2, 3...
	ThinkingWorkerCount int     `json:"thinkingWorkerCount,omitempty"` // default 3
}

// ThoughtResponse represents the structured output of a thinking step.
type ThoughtResponse struct {
	ThoughtNumber        int      `json:"thoughtNumber"`
	TotalThoughts        int      `json:"totalThoughts"`
	NextThoughtNeeded    bool     `json:"nextThoughtNeeded"`
	Branches             []string `json:"branches"`
	ThoughtHistoryLength int      `json:"thoughtHistoryLength"`
	
	// GPT Workflow response fields
	Phase                string   `json:"phase,omitempty"`
	Status               string   `json:"status,omitempty"`
	AllWorkerThoughts    []string `json:"allWorkerThoughts,omitempty"`
	SuperIdea            string   `json:"superIdea,omitempty"`
}

// SequentialThinkingServer manages the state of the thinking process.
type SequentialThinkingServer struct {
	mu                    sync.Mutex
	thoughtHistory        []ThoughtData
	branches              map[string][]ThoughtData
	disableThoughtLogging bool
	defaultWorkerCount    int
	shm                   *SharedMemoryManager
}

// NewSequentialThinkingServer creates a new instance of the server.
func NewSequentialThinkingServer() *SequentialThinkingServer {
	disableLogging := strings.ToLower(os.Getenv("DISABLE_THOUGHT_LOGGING")) == "true"
	
	workerCount := 5
	if val := os.Getenv("THINKING_WORKER_COUNT"); val != "" {
		fmt.Sscanf(val, "%d", &workerCount)
	}

	return &SequentialThinkingServer{
		thoughtHistory:        make([]ThoughtData, 0),
		branches:              make(map[string][]ThoughtData),
		disableThoughtLogging: disableLogging,
		defaultWorkerCount:    workerCount,
		shm:                   NewSharedMemoryManager(),
	}
}

// formatThought formats the thought data for logging.
func (s *SequentialThinkingServer) formatThought(data ThoughtData) string {
	var prefix string
	var context string

	phaseInfo := ""
	if data.Phase != "" {
		phaseInfo = fmt.Sprintf("[%s|W%d]", strings.ToUpper(data.Phase), data.WorkerID)
	}

	if data.IsRevision != nil && *data.IsRevision {
		prefix = color.YellowString("🔄 Revision")
		if data.RevisesThought != nil {
			context = fmt.Sprintf(" (revising thought %d)", *data.RevisesThought)
		}
	} else if data.BranchFromThought != nil {
		prefix = color.GreenString("🌿 Branch")
		id := ""
		if data.BranchID != nil {
			id = *data.BranchID
		}
		context = fmt.Sprintf(" (from thought %d, ID: %s)", *data.BranchFromThought, id)
	} else {
		prefix = color.BlueString("💭 Thought")
	}

	header := fmt.Sprintf("%s %s %d/%d%s", prefix, phaseInfo, data.ThoughtNumber, data.TotalThoughts, context)
	
	visibleHeaderLen := len(fmt.Sprintf("Thought %s %d/%d%s", phaseInfo, data.ThoughtNumber, data.TotalThoughts, context)) + 3
	
	contentLen := len(data.Thought)
	maxLen := visibleHeaderLen
	if contentLen > maxLen {
		maxLen = contentLen
	}
	
	border := strings.Repeat("─", maxLen+4)

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("┌%s┐\n", border))
	sb.WriteString(fmt.Sprintf("│ %s%s │\n", header, strings.Repeat(" ", maxLen-visibleHeaderLen)))
	sb.WriteString(fmt.Sprintf("├%s┤\n", border))
	sb.WriteString(fmt.Sprintf("│ %s%s │\n", data.Thought, strings.Repeat(" ", maxLen-contentLen)))
	sb.WriteString(fmt.Sprintf("└%s┘", border))

	return sb.String()
}

// ProcessThought processes a new thinking step and returns the response.
func (s *SequentialThinkingServer) ProcessThought(input ThoughtData) (ThoughtResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set defaults
	if input.Phase == "" {
		input.Phase = "gather"
	}

	// Auto-reset if this is the start of a new thinking session (ThoughtNumber == 1)
	// and not a branch or revision.
	isRevision := input.IsRevision != nil && *input.IsRevision
	if input.ThoughtNumber == 1 && input.BranchFromThought == nil && input.RevisesThought == nil && !isRevision {
		shouldReset := false
		if len(s.thoughtHistory) == 0 {
			shouldReset = true
		} else {
			// Check if we already have this thought number and worker ID in history.
			// If we do, it means this is a new session starting over.
			for _, t := range s.thoughtHistory {
				if t.ThoughtNumber == input.ThoughtNumber && t.WorkerID == input.WorkerID {
					shouldReset = true
					break
				}
			}

			// Also reset if the last thought was finished
			if !shouldReset {
				lastThought := s.thoughtHistory[len(s.thoughtHistory)-1]
				if !lastThought.NextThoughtNeeded {
					shouldReset = true
				}
			}
		}

		if shouldReset {
			fmt.Fprint(os.Stderr, color.CyanString("🧹 Auto-clearing shared memory for new thinking session...\n"))
			s.shm.ClearAll()
			s.thoughtHistory = make([]ThoughtData, 0)
			s.branches = make(map[string][]ThoughtData)
		}
	}

	// Validate phase
	validPhases := map[string]bool{"gather": true, "process": true, "test": true}
	if !validPhases[strings.ToLower(input.Phase)] {
		return ThoughtResponse{}, fmt.Errorf("invalid phase: %s. Must be 'gather', 'process', or 'test'", input.Phase)
	}

	if input.ThinkingWorkerCount == 0 {
		input.ThinkingWorkerCount = s.defaultWorkerCount
	}
	if input.WorkerID == 0 {
		input.WorkerID = 1
	}

	// Adjust totalThoughts if thoughtNumber exceeds it
	if input.ThoughtNumber > input.TotalThoughts {
		input.TotalThoughts = input.ThoughtNumber
	}

	s.thoughtHistory = append(s.thoughtHistory, input)

	// Save to shared memory
	if err := s.shm.SaveThought(input.Phase, input.WorkerID, input); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save to shm: %v\n", err)
	}

	if input.BranchFromThought != nil && input.BranchID != nil {
		id := *input.BranchID
		if _, ok := s.branches[id]; !ok {
			s.branches[id] = make([]ThoughtData, 0)
		}
		s.branches[id] = append(s.branches[id], input)
	}

	if !s.disableThoughtLogging {
		formatted := s.formatThought(input)
		fmt.Fprintln(os.Stderr, formatted)
	}

	// Cleanup shared memory if the task is done (NextThoughtNeeded is false)
	if !input.NextThoughtNeeded {
		fmt.Fprint(os.Stderr, color.GreenString("✅ Task completed. Cleaning up shared memory...\n"))
		s.shm.ClearAll()
	}

	branches := make([]string, 0, len(s.branches))
	for k := range s.branches {
		branches = append(branches, k)
	}

	resp := ThoughtResponse{
		ThoughtNumber:        input.ThoughtNumber,
		TotalThoughts:        input.TotalThoughts,
		NextThoughtNeeded:    input.NextThoughtNeeded,
		Branches:             branches,
		ThoughtHistoryLength: len(s.thoughtHistory),
		Phase:                input.Phase,
	}

	// GPT Workflow Logic
	if input.Phase == "gather" {
		thoughts, _ := s.shm.GetPhaseThoughts("gather")
		if len(thoughts) >= input.ThinkingWorkerCount {
			resp.Status = "all_workers_finished"
			var allThoughts []string
			for _, t := range thoughts {
				allThoughts = append(allThoughts, fmt.Sprintf("Worker %d: %s", t.WorkerID, t.Thought))
			}
			resp.AllWorkerThoughts = allThoughts
			resp.SuperIdea = s.synthesizeSuperIdea(thoughts)
		} else {
			resp.Status = fmt.Sprintf("waiting_for_workers (%d/%d)", len(thoughts), input.ThinkingWorkerCount)
		}
	} else {
		resp.Status = "completed"
	}

	return resp, nil
}

func (s *SequentialThinkingServer) synthesizeSuperIdea(thoughts []ThoughtData) string {
	// Simple synthesis: combine key points. 
	// In a real scenario, this might be another LLM call, 
	// but here we provide a structured summary to help the LLM synthesize.
	var sb strings.Builder
	sb.WriteString("Super Idea Synthesis:\n")
	sb.WriteString("Combined the following perspectives:\n")
	for _, t := range thoughts {
		sb.WriteString(fmt.Sprintf("- Perspective from Worker %d: %s\n", t.WorkerID, t.Thought))
	}
	sb.WriteString("\nRecommended Action: Evaluate the strengths of each worker's proposal and merge them into a single, robust implementation plan for the 'Process' phase.")
	return sb.String()
}
