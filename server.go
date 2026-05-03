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
	Context             string `json:"context,omitempty"`             // Discovered tools, environment, etc.
	GenerateDiagram     *bool  `json:"generateDiagram,omitempty"`     // Opt-in flag for Mermaid diagram

	// Security & Performance fields
	IsPrivate  bool   `json:"isPrivate,omitempty"`  // Zero-Knowledge: Redact from synthesis
	IsTainted  bool   `json:"isTainted,omitempty"`  // Taint Analysis: Mark as untrusted
	Complexity string `json:"complexity,omitempty"` // Metadata for dynamic scaling
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
	Context           string   `json:"context,omitempty"`
	Flowchart         string   `json:"flowchart,omitempty"` // Markdown-native ASCII flowchart

	// Security & Performance response fields
	IsTainted            bool `json:"isTainted,omitempty"`
	SuggestedWorkerCount int  `json:"suggestedWorkerCount,omitempty"`
}

// SequentialThinkingServer manages the state of the thinking process.
type SequentialThinkingServer struct {
	mu                    sync.Mutex
	thoughtHistory        []ThoughtData
	branches              map[string][]ThoughtData
	disableThoughtLogging bool
	defaultWorkerCount    int
	maxWorkerCount        int
	defaultEnableDiagram  bool
	currentFlowchartFile  string
	shm                   *SharedMemoryManager
}

// NewSequentialThinkingServer creates a new instance of the server.
func NewSequentialThinkingServer(workerCount int, maxWorkerCount int, shmRoot string, defaultEnableDiagram bool) *SequentialThinkingServer {
	disableLogging := strings.ToLower(os.Getenv("DISABLE_THOUGHT_LOGGING")) == "true"

	if workerCount <= 0 {
		workerCount = 5
		if val := os.Getenv("THINKING_WORKER_COUNT"); val != "" {
			fmt.Sscanf(val, "%d", &workerCount)
		}
	}

	if maxWorkerCount <= 0 {
		maxWorkerCount = 10
		if val := os.Getenv("MAX_THINKING_WORKER_COUNT"); val != "" {
			fmt.Sscanf(val, "%d", &maxWorkerCount)
		}
	}

	// Ensure default doesn't exceed max
	if workerCount > maxWorkerCount {
		workerCount = maxWorkerCount
	}

	return &SequentialThinkingServer{
		thoughtHistory:        make([]ThoughtData, 0),
		branches:              make(map[string][]ThoughtData),
		disableThoughtLogging: disableLogging,
		defaultWorkerCount:    workerCount,
		maxWorkerCount:        maxWorkerCount,
		defaultEnableDiagram:  defaultEnableDiagram,
		shm:                   NewSharedMemoryManager(shmRoot),
	}
}

func (s *SequentialThinkingServer) getAvailableFilename() string {
	base := "deepthinking-flow.md"
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return base
	}

	i := 1
	for {
		filename := fmt.Sprintf("deepthinking-%d-flow.md", i)
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			return filename
		}
		i++
	}
}

func (s *SequentialThinkingServer) saveFlowchartLocked(content string) {
	if content == "" {
		return
	}

	if s.currentFlowchartFile == "" {
		s.currentFlowchartFile = s.getAvailableFilename()
	}
	filename := s.currentFlowchartFile

	err := os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error saving flowchart to %s: %v\n", filename, err)
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

	// DoS Protection: Limit input sizes
	const maxInputSize = 100 * 1024 // 100KB
	if len(input.Thought) > maxInputSize {
		return ThoughtResponse{}, fmt.Errorf("thought exceeds maximum size of %d bytes", maxInputSize)
	}
	if len(input.Context) > maxInputSize {
		return ThoughtResponse{}, fmt.Errorf("context exceeds maximum size of %d bytes", maxInputSize)
	}

	// Deep Redaction: Redact secrets at the entry point to prevent storage in memory/disk
	// and to prevent sending them back to the LLM in the synthesis phase.
	input.Thought = redact(input.Thought)
	input.Context = redact(input.Context)

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
	if input.ThinkingWorkerCount > s.maxWorkerCount {
		input.ThinkingWorkerCount = s.maxWorkerCount
	}
	if input.WorkerID == 0 {
		input.WorkerID = 1
	}

	// --- STRICT GPT WORKFLOW ENFORCEMENT ---
	if input.Phase == "process" {
		gatherThoughts, _ := s.shm.GetPhaseThoughts("gather")
		if len(gatherThoughts) < input.ThinkingWorkerCount {
			return ThoughtResponse{}, fmt.Errorf("STRICT WORKFLOW VIOLATION: Cannot enter 'process' phase. 'gather' phase is incomplete. You must gather perspectives from %d workers (currently have %d). FIX: Call this tool again with phase='gather' and workerId=%d", input.ThinkingWorkerCount, len(gatherThoughts), len(gatherThoughts)+1)
		}
		// Combine thoughts and clear gather phase to prevent reuse
		s.shm.ClearPhase("gather")
	} else if input.Phase == "test" {
		// Check in-memory history to allow SHM cleanup of previous phases
		hasProcess := false
		for _, t := range s.thoughtHistory {
			if t.Phase == "process" {
				hasProcess = true
				break
			}
		}
		if !hasProcess {
			return ThoughtResponse{}, fmt.Errorf("STRICT WORKFLOW VIOLATION: Cannot enter 'test' phase. 'process' phase is incomplete. You must process the gathered ideas first using phase='process'. FIX: Call this tool again with phase='process'")
		}
		// Clear process phase from SHM to avoid leakage as soon as we enter test
		s.shm.ClearPhase("process")
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
		Context:              input.Context,
	}

	// Taint Propagation
	isTainted := input.IsTainted
	if !isTainted {
		for _, t := range s.thoughtHistory {
			if t.IsTainted {
				isTainted = true
				break
			}
		}
	}
	resp.IsTainted = isTainted

	// Dynamic Scaling Suggestion
	if input.ThoughtNumber == 1 {
		suggested := 5
		length := len(input.Thought)
		if length > 500 {
			// Incremental scaling: +1 worker for every 200 characters above 500
			suggested = 6 + (length-500)/200
		}

		if suggested > s.maxWorkerCount {
			suggested = s.maxWorkerCount
		}
		resp.SuggestedWorkerCount = suggested
	}

	// Generate Markdown flowchart if requested or enabled by default
	shouldGenerateDiagram := s.defaultEnableDiagram
	if input.GenerateDiagram != nil {
		shouldGenerateDiagram = *input.GenerateDiagram
	}

	if shouldGenerateDiagram {
		resp.Flowchart = s.generateMarkdownFlowchart()
		s.saveFlowchartLocked(resp.Flowchart)
	}

	// GPT Workflow Logic
	if input.Phase == "gather" {
		thoughts, _ := s.shm.GetPhaseThoughts("gather")
		if len(thoughts) >= input.ThinkingWorkerCount {
			resp.Status = "all_workers_finished"
			var allThoughts []string
			var combinedContext []string
			for _, t := range thoughts {
				thoughtText := t.Thought
				if t.IsPrivate {
					thoughtText = "[PRIVATE THOUGHT - REDACTED FOR SECURITY]"
				}
				allThoughts = append(allThoughts, fmt.Sprintf("Worker %d: %s", t.WorkerID, thoughtText))
				if t.Context != "" {
					combinedContext = append(combinedContext, fmt.Sprintf("Worker %d Context: %s", t.WorkerID, t.Context))
				}
			}
			resp.AllWorkerThoughts = allThoughts
			resp.SuperIdea = s.synthesizeSuperIdea(thoughts, input.Track, strings.Join(combinedContext, "\n"), isTainted)
		} else {
			resp.Status = fmt.Sprintf("waiting_for_workers (%d/%d)", len(thoughts), input.ThinkingWorkerCount)
		}
	} else {
		resp.Status = "completed"
	}

	return resp, nil
}

func (s *SequentialThinkingServer) synthesizeSuperIdea(thoughts []ThoughtData, track string, context string, isTainted bool) string {
	var sb strings.Builder
	sb.WriteString("🚀 SUPER IDEA SYNTHESIS\n")
	sb.WriteString("=======================\n\n")

	if isTainted {
		sb.WriteString("⚠️ WARNING: This strategy was derived from untrusted (tainted) thoughts. Proceed with caution.\n\n")
	}

	if track != "" {
		sb.WriteString(fmt.Sprintf("🛤️ TRACK: %s\n\n", strings.ToUpper(track)))
	}

	if context != "" {
		sb.WriteString("🌍 ENVIRONMENTAL CONTEXT:\n")
		sb.WriteString(context)
		sb.WriteString("\n\n")
	}

	sb.WriteString("The following perspectives have been gathered and analyzed from multiple thinking workers:\n\n")

	for _, t := range thoughts {
		sb.WriteString(fmt.Sprintf("📍 Worker %d Perspective:\n", t.WorkerID))
		thoughtText := t.Thought
		if t.IsPrivate {
			thoughtText = "[PRIVATE THOUGHT - REDACTED FOR SECURITY]"
		}
		sb.WriteString(fmt.Sprintf("   \"%s\"\n\n", thoughtText))
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
	sb.WriteString("\n\n💡 TIP: If you want to visualize this thinking path, set 'generateDiagram: true' in your next step.")
	sb.WriteString("\n\n➡️ NEXT STEP: Proceed to the 'PROCESS' phase to implement your synthesized strategy.")

	return redact(sb.String())
}

func (s *SequentialThinkingServer) wrapText(text string, width int) []string {
	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	currentLine := words[0]
	for _, word := range words[1:] {
		if len(currentLine)+1+len(word) > width {
			lines = append(lines, currentLine)
			currentLine = word
		} else {
			currentLine += " " + word
		}
	}
	lines = append(lines, currentLine)
	return lines
}

func (s *SequentialThinkingServer) generateMarkdownFlowchart() string {
	var sb strings.Builder
	sb.WriteString("```text\n")
	sb.WriteString("🧠 DEEPTHINKING FLOWCHART\n")
	sb.WriteString("=========================\n\n")

	boxWidth := 60
	currentPhase := ""
	for i, t := range s.thoughtHistory {
		// Phase Header
		if t.Phase != currentPhase {
			currentPhase = t.Phase
			sb.WriteString(fmt.Sprintf("\n--- %s PHASE ---\n", strings.ToUpper(currentPhase)))
		}

		// Thought Box
		border := "+" + strings.Repeat("-", boxWidth-2) + "+"
		sb.WriteString(border + "\n")

		// Header: T# (W#)
		header := fmt.Sprintf("T%d (W%d)", t.ThoughtNumber, t.WorkerID)
		if t.IsRevision != nil && *t.IsRevision {
			header += " [REV]"
		}
		leftPad := (boxWidth - 2 - len(header)) / 2
		rightPad := boxWidth - 2 - len(header) - leftPad
		sb.WriteString(fmt.Sprintf("|%s%s%s|\n", strings.Repeat(" ", leftPad), header, strings.Repeat(" ", rightPad)))
		
		sb.WriteString("|" + strings.Repeat("-", boxWidth-2) + "|\n")

		// Body: Thought Snippet
		thoughtLines := s.wrapText(t.Thought, boxWidth-6)
		if len(thoughtLines) > 3 {
			thoughtLines = append(thoughtLines[:3], "...")
		}
		for _, line := range thoughtLines {
			sb.WriteString(fmt.Sprintf("|  %-*s  |\n", boxWidth-6, line))
		}

		// Footer: Context/Tools
		if t.Context != "" {
			sb.WriteString("|" + strings.Repeat(".", boxWidth-2) + "|\n")
			contextLines := s.wrapText("Context: "+t.Context, boxWidth-6)
			if len(contextLines) > 2 {
				contextLines = append(contextLines[:2], "...")
			}
			for _, line := range contextLines {
				sb.WriteString(fmt.Sprintf("|  %-*s  |\n", boxWidth-6, line))
			}
		}

		sb.WriteString(border + "\n")

		// Annotations (Branch/Revision)
		if t.BranchFromThought != nil {
			sb.WriteString(fmt.Sprintf("  ^-- [Branch from T%d", *t.BranchFromThought))
			if t.BranchID != nil {
				sb.WriteString(fmt.Sprintf(", ID: %s", *t.BranchID))
			}
			sb.WriteString("]\n")
		}
		if t.IsRevision != nil && *t.IsRevision && t.RevisesThought != nil {
			sb.WriteString(fmt.Sprintf("  ^-- [Revises T%d]\n", *t.RevisesThought))
		}

		// Arrow to next
		if i < len(s.thoughtHistory)-1 {
			sb.WriteString(strings.Repeat(" ", boxWidth/2) + "|\n")
			sb.WriteString(strings.Repeat(" ", boxWidth/2) + "v\n")
		}
	}

	sb.WriteString("\n=========================\n")
	sb.WriteString("```")
	return sb.String()
}
