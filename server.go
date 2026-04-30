package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/fatih/color"
)

var (
	reEmail  = regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
	reOpenAI = regexp.MustCompile(`\bsk-[a-zA-Z0-9]{20,}\b`)
	reAWS    = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	reHex    = regexp.MustCompile(`\b[a-fA-F0-9]{32,}\b`)
	reTrack  = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)
	reGitHub = regexp.MustCompile(`\bgh[pso]_[a-zA-Z0-9]{36}\b`)
	reGoogle = regexp.MustCompile(`\bAIza[0-9A-Za-z\-_]{35}\b`)
	reSlack  = regexp.MustCompile(`\bxox[baprs]-[0-9a-zA-Z\-]{10,64}\b`)
	rePrivKey = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
)

func redact(s string) string {
	s = reEmail.ReplaceAllString(s, "[REDACTED EMAIL]")
	s = reOpenAI.ReplaceAllString(s, "[REDACTED OPENAI KEY]")
	s = reAWS.ReplaceAllString(s, "[REDACTED AWS ID]")
	s = reHex.ReplaceAllString(s, "[REDACTED KEY]")
	s = reGitHub.ReplaceAllString(s, "[REDACTED GITHUB TOKEN]")
	s = reGoogle.ReplaceAllString(s, "[REDACTED GOOGLE KEY]")
	s = reSlack.ReplaceAllString(s, "[REDACTED SLACK TOKEN]")
	s = rePrivKey.ReplaceAllString(s, "[REDACTED PRIVATE KEY]")
	return s
}

func validateTrack(track string) error {
	if track == "" {
		return nil
	}
	if !reTrack.MatchString(track) {
		return fmt.Errorf("invalid track name: %s. Must be 1-32 characters and contain only alphanumeric characters, underscores, or hyphens", track)
	}
	return nil
}

// ThoughtData represents the input for a single thinking step.
type ThoughtData struct {
	Thought           string  `json:"thought"`
	ThoughtNumber     int     `json:"thoughtNumber"`
	TotalThoughts     int     `json:"totalThoughts"`
	NextThoughtNeeded bool    `json:"nextThoughtNeeded"`
	IsRevision        *bool   `json:"isRevision,omitempty"`
	RevisesThought    *int    `json:"revisesThought,omitempty"`
	BranchFromThought *int    `json:"branchFromThought,omitempty"`
	BranchID          *string `json:"branchId,omitempty"`
	NeedsMoreThoughts *bool   `json:"needsMoreThoughts,omitempty"`

	// GPT Workflow fields
	Phase               string `json:"phase,omitempty"`               // "gather", "process", "test"
	WorkerID            int    `json:"workerId,omitempty"`            // 1, 2, 3...
	ThinkingWorkerCount int    `json:"thinkingWorkerCount,omitempty"` // default 5
	Track               string `json:"track,omitempty"`               // "bug-fix", "feature", "security", etc.
}

// ThoughtResponse represents the structured output of a thinking step.
type ThoughtResponse struct {
	ThoughtNumber        int      `json:"thoughtNumber"`
	TotalThoughts        int      `json:"totalThoughts"`
	NextThoughtNeeded    bool     `json:"nextThoughtNeeded"`
	Branches             []string `json:"branches"`
	ThoughtHistoryLength int      `json:"thoughtHistoryLength"`
	SessionID            string   `json:"sessionId"`

	// GPT Workflow response fields
	Phase             string   `json:"phase,omitempty"`
	Status            string   `json:"status,omitempty"`
	AllWorkerThoughts []string `json:"allWorkerThoughts,omitempty"`
	SuperIdea         string   `json:"superIdea,omitempty"`
	Track             string   `json:"track,omitempty"`
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
func NewSequentialThinkingServer(workerCount int, shmRoot string) *SequentialThinkingServer {
	disableLogging := strings.ToLower(os.Getenv("DISABLE_THOUGHT_LOGGING")) == "true"

	if workerCount <= 0 {
		workerCount = 5
		if val := os.Getenv("THINKING_WORKER_COUNT"); val != "" {
			fmt.Sscanf(val, "%d", &workerCount)
		}
	}

	return &SequentialThinkingServer{
		thoughtHistory:        make([]ThoughtData, 0),
		branches:              make(map[string][]ThoughtData),
		disableThoughtLogging: disableLogging,
		defaultWorkerCount:    workerCount,
		shm:                   NewSharedMemoryManager(shmRoot),
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

	thought := redact(data.Thought)
	contentLen := len(thought)
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
	sb.WriteString(fmt.Sprintf("│ %s%s │\n", thought, strings.Repeat(" ", maxLen-contentLen)))
	sb.WriteString(fmt.Sprintf("└%s┘", border))

	return sb.String()
}

// ProcessThought processes a new thinking step and returns the response.
func (s *SequentialThinkingServer) ProcessThought(input ThoughtData) (ThoughtResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deep Redaction: Redact secrets at the entry point to prevent storage in memory/disk
	// and to prevent sending them back to the LLM in the synthesis phase.
	input.Thought = redact(input.Thought)

	phaseProvided := input.Phase != ""

	// Set defaults
	if input.Phase == "" {
		input.Phase = "gather"
	}

	// Auto-increment thought number if not provided (0)
	if input.ThoughtNumber <= 0 {
		input.ThoughtNumber = len(s.thoughtHistory) + 1
	}

	// Set total thoughts if not provided or less than thought number
	if input.TotalThoughts < input.ThoughtNumber {
		input.TotalThoughts = input.ThoughtNumber
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

			// Also reset if the last thought was finished, but only if we are not in gather phase
			if !shouldReset && input.Phase != "gather" {
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
	input.Phase = strings.ToLower(input.Phase)
	validPhases := map[string]bool{"gather": true, "process": true, "test": true}
	if !validPhases[input.Phase] {
		return ThoughtResponse{}, fmt.Errorf("invalid phase: %s. Must be 'gather', 'process', or 'test'", input.Phase)
	}

	// Validate track
	if err := validateTrack(input.Track); err != nil {
		return ThoughtResponse{}, err
	}

	if input.ThinkingWorkerCount == 0 {
		input.ThinkingWorkerCount = s.defaultWorkerCount
	}
	if input.WorkerID == 0 {
		input.WorkerID = 1
	}

	// --- STRICT GPT WORKFLOW ENFORCEMENT ---
	if input.Phase == "process" {
		gatherThoughts, _ := s.shm.GetPhaseThoughts("gather")
		if len(gatherThoughts) < input.ThinkingWorkerCount {
			return ThoughtResponse{}, fmt.Errorf("STRICT WORKFLOW VIOLATION: Cannot enter 'process' phase. 'gather' phase is incomplete. You must gather perspectives from %d workers (currently have %d). Use phase='gather' with different workerIds", input.ThinkingWorkerCount, len(gatherThoughts))
		}
		// Combine thoughts and clear gather phase to prevent reuse
		s.shm.ClearPhase("gather")
	} else if input.Phase == "test" {
		processThoughts, _ := s.shm.GetPhaseThoughts("process")
		if len(processThoughts) == 0 {
			return ThoughtResponse{}, fmt.Errorf("STRICT WORKFLOW VIOLATION: Cannot enter 'test' phase. 'process' phase is incomplete. You must process the gathered ideas first using phase='process'")
		}
	}
	// ---------------------------------------

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
	// Only clear if we are in the 'test' phase or if no phase was specified (standard mode)
	if !input.NextThoughtNeeded {
		if input.Phase == "test" || !phaseProvided {
			fmt.Fprint(os.Stderr, color.GreenString("✅ Task completed. Cleaning up shared memory...\n"))
			s.shm.ClearAll()
		}
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
		SessionID:            s.shm.GetSessionID(),
		Track:                input.Track,
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
			resp.SuperIdea = s.synthesizeSuperIdea(thoughts, input.Track)
		} else {
			resp.Status = fmt.Sprintf("waiting_for_workers (%d/%d)", len(thoughts), input.ThinkingWorkerCount)
		}
	} else {
		resp.Status = "completed"
	}

	return resp, nil
}

func (s *SequentialThinkingServer) synthesizeSuperIdea(thoughts []ThoughtData, track string) string {
	var sb strings.Builder
	sb.WriteString("🚀 SUPER IDEA SYNTHESIS\n")
	sb.WriteString("=======================\n\n")

	if track != "" {
		sb.WriteString(fmt.Sprintf("🛤️ TRACK: %s\n\n", strings.ToUpper(track)))
	}

	sb.WriteString("The following perspectives have been gathered and analyzed from multiple thinking workers:\n\n")

	for _, t := range thoughts {
		sb.WriteString(fmt.Sprintf("📍 Worker %d Perspective:\n", t.WorkerID))
		sb.WriteString(fmt.Sprintf("   \"%s\"\n\n", t.Thought))
	}

	sb.WriteString("🎯 LLM ACTION REQUIRED: INTEGRATED STRATEGY SYNTHESIS\n")
	sb.WriteString("----------------------------------------------------\n")
	sb.WriteString("You are now acting as the 'Strategic Orchestrator'. Your task is to analyze the diverse perspectives above and synthesize them into a single, high-fidelity integrated strategy. ")

	switch strings.ToLower(track) {
	case "bug-fix":
		sb.WriteString("As this is a 'BUG-FIX' track, prioritize root cause isolation, regression prevention, and ensuring minimal side effects on the existing codebase.")
	case "feature":
		sb.WriteString("As this is a 'FEATURE' track, prioritize scalability, optimal user experience, and alignment with the existing architectural patterns.")
	case "security":
		sb.WriteString("As this is a 'SECURITY' track, prioritize threat modeling, attack surface reduction, and the principle of defense-in-depth.")
	default:
		if track != "" {
			sb.WriteString(fmt.Sprintf("As this is a '%s' track, prioritize the core objectives of this mode and ensure high-quality engineering standards.", strings.ToUpper(track)))
		} else {
			sb.WriteString("Synthesize a unified approach that incorporates the unique strengths of each proposal while resolving any contradictions or trade-offs identified.")
		}
	}

	sb.WriteString("\n\nYour synthesis should be comprehensive and provide a clear path forward for the 'PROCESS' phase.")
	sb.WriteString("\n\n➡️ NEXT STEP: Proceed to the 'PROCESS' phase to implement your synthesized strategy.")

	return redact(sb.String())
}
