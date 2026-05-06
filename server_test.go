package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeepThinking(t *testing.T) {
	server := NewDeepThinkingServer(0, 10, "", false)
	defer os.RemoveAll(server.shm.shmPath)
	// Ensure we start fresh
	server.shm.ClearAll()

	t.Run("Basic Thought", func(t *testing.T) {
		thought := "Initial analysis"
		nextNeeded := true
		args := ThoughtData{
			Thought:           thought,
			NextThoughtNeeded: nextNeeded,
		}

		resp, err := server.ProcessThought(args)
		if err != nil {
			t.Fatalf("ProcessThought failed: %v", err)
		}

		if resp.ThoughtNumber != 1 {
			t.Errorf("Expected ThoughtNumber 1, got %d", resp.ThoughtNumber)
		}
		if resp.TotalThoughts != 1 {
			t.Errorf("Expected TotalThoughts 1, got %d", resp.TotalThoughts)
		}
		if !resp.NextThoughtNeeded {
			t.Error("Expected NextThoughtNeeded to be true")
		}
	})

	t.Run("Auto-increment ThoughtNumber", func(t *testing.T) {
		nextNeeded := true
		args := ThoughtData{
			Thought:           "Second thought",
			NextThoughtNeeded: nextNeeded,
		}

		resp, err := server.ProcessThought(args)
		if err != nil {
			t.Fatalf("ProcessThought failed: %v", err)
		}

		if resp.ThoughtNumber != 2 {
			t.Errorf("Expected ThoughtNumber 2, got %d", resp.ThoughtNumber)
		}
	})

	t.Run("Gather Phase Workflow", func(t *testing.T) {
		// Reset for clean test
		server.shm.ClearAll()
		server.thoughtHistory = nil

		workerCount := 3
		server.defaultWorkerCount = workerCount
		nextNeeded := true

		for i := 1; i <= workerCount; i++ {
			args := ThoughtData{
				Thought:             "Gathering info",
				Phase:               "gather",
				WorkerID:            i,
				ThinkingWorkerCount: workerCount,
				NextThoughtNeeded:   nextNeeded,
			}
			resp, err := server.ProcessThought(args)
			if err != nil {
				t.Fatalf("Worker %d failed: %v", i, err)
			}

			if i < workerCount {
				if resp.Status != "waiting_for_workers (1/3)" && resp.Status != "waiting_for_workers (2/3)" {
					t.Errorf("Unexpected status for worker %d: %s", i, resp.Status)
				}
			} else {
				if resp.Status != "all_workers_finished" {
					t.Errorf("Expected all_workers_finished, got %s", resp.Status)
				}
				if resp.SuperIdea == "" {
					t.Error("Expected SuperIdea to be generated")
				}
			}
		}
	})

	t.Run("Gather Phase Workflow - 5 Workers", func(t *testing.T) {
		// Reset for clean test
		server.shm.ClearAll()
		server.thoughtHistory = nil

		workerCount := 5
		server.defaultWorkerCount = workerCount
		nextNeeded := false // Test with false to ensure fix works

		for i := 1; i <= workerCount; i++ {
			args := ThoughtData{
				Thought:             "Gathering info",
				Phase:               "gather",
				WorkerID:            i,
				ThinkingWorkerCount: workerCount,
				NextThoughtNeeded:   nextNeeded,
			}
			resp, err := server.ProcessThought(args)
			if err != nil {
				t.Fatalf("Worker %d failed: %v", i, err)
			}

			if i < workerCount {
				expectedPrefix := "waiting_for_workers"
				if len(resp.Status) < len(expectedPrefix) || resp.Status[:len(expectedPrefix)] != expectedPrefix {
					t.Errorf("Unexpected status for worker %d: %s", i, resp.Status)
				}
			} else {
				if resp.Status != "all_workers_finished" {
					t.Errorf("Expected all_workers_finished, got %s", resp.Status)
				}
				if len(resp.AllWorkerThoughts) != 5 {
					t.Errorf("Expected 5 worker thoughts, got %d", len(resp.AllWorkerThoughts))
				}
				if resp.SuperIdea == "" {
					t.Error("Expected SuperIdea to be generated")
				}
			}
		}
	})

	t.Run("Strict Workflow Enforcement", func(t *testing.T) {
		server.shm.ClearAll()
		server.thoughtHistory = nil
		nextNeeded := true

		// 1. Try to enter process before gather is done -> should fail
		argsProcess := ThoughtData{
			Thought:             "Processing without gathering",
			Phase:               "process",
			WorkerID:            1,
			ThinkingWorkerCount: 3,
			NextThoughtNeeded:   nextNeeded,
		}
		_, err := server.ProcessThought(argsProcess)
		if err == nil {
			t.Fatal("Expected error when entering process phase without gathering")
		} else if !strings.Contains(err.Error(), "💡 NUDGE: Workflow Requirement") {
			t.Errorf("Unexpected error message: %v", err.Error())
		}

		// 2. Do a partial gather
		argsGather := ThoughtData{
			Thought:             "Gather 1",
			Phase:               "gather",
			WorkerID:            1,
			ThinkingWorkerCount: 2,
			NextThoughtNeeded:   nextNeeded,
		}
		server.ProcessThought(argsGather)

		// 3. Try process again -> should fail
		argsProcess.ThinkingWorkerCount = 2
		_, err = server.ProcessThought(argsProcess)
		if err == nil {
			t.Error("Expected error when entering 'process' phase with partial 'gather'")
		}

		// 4. Try test without process -> should fail
		argsTest := ThoughtData{
			Thought:           "Testing",
			Phase:             "test",
			WorkerID:          1,
			NextThoughtNeeded: nextNeeded,
		}
		_, err = server.ProcessThought(argsTest)
		if err == nil {
			t.Fatal("Expected error when entering test phase without processing")
		} else if !strings.Contains(err.Error(), "💡 NUDGE: Workflow Requirement") {
			t.Errorf("Unexpected error message: %v", err.Error())
		}

		// 5. Complete gather
		argsGather.WorkerID = 2
		server.ProcessThought(argsGather)

		// 6. Try process -> should succeed
		_, err = server.ProcessThought(argsProcess)
		if err != nil {
			t.Errorf("Expected success when entering 'process' phase after 'gather' is complete, got: %v", err)
		}

		// 7. Try test -> should succeed
		_, err = server.ProcessThought(argsTest)
		if err != nil {
			t.Errorf("Expected success when entering 'test' phase after 'process', got: %v", err)
		}
	})

	t.Run("Completion and Cleanup", func(t *testing.T) {
		nextNeeded := false
		args := ThoughtData{
			Thought:           "Final conclusion",
			NextThoughtNeeded: nextNeeded,
		}

		_, err := server.ProcessThought(args)
		if err != nil {
			t.Fatalf("ProcessThought failed: %v", err)
		}

		// Check if shm is cleared (GetPhaseThoughts should return empty)
		thoughts, _ := server.shm.GetPhaseThoughts("gather")
		if len(thoughts) > 0 {
			t.Error("Shared memory was not cleared after completion")
		}
	})
}

func TestResetThinking(t *testing.T) {
	server := NewDeepThinkingServer(0, 10, "", false)
	defer os.RemoveAll(server.shm.shmPath)
	nextNeeded := true
	server.ProcessThought(ThoughtData{Thought: "Test", NextThoughtNeeded: nextNeeded})

	err := server.shm.ClearAll()
	if err != nil {
		t.Fatalf("ClearAll failed: %v", err)
	}

	server.mu.Lock()
	server.thoughtHistory = nil
	server.mu.Unlock()

	if len(server.thoughtHistory) != 0 {
		t.Error("History not cleared")
	}
}

func TestMultipleThoughtsPerWorker(t *testing.T) {
	server := NewDeepThinkingServer(0, 10, "", false)
	defer os.RemoveAll(server.shm.shmPath)
	server.shm.ClearAll()
	server.thoughtHistory = nil

	workerCount := 1
	server.defaultWorkerCount = workerCount
	nextNeeded := true

	// Worker 1 submits first thought
	args1 := ThoughtData{
		Thought:             "Worker 1 first thought",
		Phase:               "gather",
		WorkerID:            1,
		ThinkingWorkerCount: workerCount,
		NextThoughtNeeded:   nextNeeded,
	}
	server.ProcessThought(args1)

	// Worker 1 submits second thought (e.g. they realized they needed to add more)
	args2 := ThoughtData{
		Thought:             "Worker 1 second thought",
		Phase:               "gather",
		WorkerID:            1,
		ThinkingWorkerCount: workerCount,
		NextThoughtNeeded:   nextNeeded,
	}
	server.ProcessThought(args2)

	// Worker 1 submits third thought
	args3 := ThoughtData{
		Thought:             "Worker 1 third thought",
		Phase:               "gather",
		WorkerID:            1,
		ThinkingWorkerCount: workerCount,
		NextThoughtNeeded:   nextNeeded,
	}
	resp, err := server.ProcessThought(args3)
	
	if err != nil {
		t.Fatalf("ProcessThought failed: %v", err)
	}

	// Because worker count is 1, it should immediately process all 3 thoughts from Worker 1
	if len(resp.AllWorkerThoughts) != 3 {
		t.Errorf("Expected 3 thoughts in AllWorkerThoughts, got %d", len(resp.AllWorkerThoughts))
	}
	
	// Check that we can read them back out from SHM and confirm there are 3 separate files
	thoughts, _ := server.shm.GetPhaseThoughts("gather")
	if len(thoughts) != 3 {
		t.Errorf("Expected 3 thoughts physically saved in SHM, got %d", len(thoughts))
	}
}

func TestProcessPhaseTransition(t *testing.T) {
	server := NewDeepThinkingServer(0, 10, "", false)
	defer os.RemoveAll(server.shm.shmPath)
	server.shm.ClearAll()
	server.thoughtHistory = nil

	workerCount := 2
	server.defaultWorkerCount = workerCount
	nextNeeded := true

	// 1. Gather Phase - Worker 1
	server.ProcessThought(ThoughtData{
		Thought:             "Worker 1 idea",
		Phase:               "gather",
		WorkerID:            1,
		ThinkingWorkerCount: workerCount,
		NextThoughtNeeded:   nextNeeded,
	})

	// Gather Phase - Worker 2
	server.ProcessThought(ThoughtData{
		Thought:             "Worker 2 idea",
		Phase:               "gather",
		WorkerID:            2,
		ThinkingWorkerCount: workerCount,
		NextThoughtNeeded:   nextNeeded,
	})

	// Verify gather phase files exist
	gatherThoughts, _ := server.shm.GetPhaseThoughts("gather")
	if len(gatherThoughts) != 2 {
		t.Fatalf("Expected 2 gather thoughts, got %d", len(gatherThoughts))
	}

	// 2. Process Phase Transition
	processArgs := ThoughtData{
		Thought:             "Synthesizing gathered ideas",
		Phase:               "process",
		WorkerID:            1,
		ThinkingWorkerCount: workerCount,
		NextThoughtNeeded:   nextNeeded,
	}
	
	_, err := server.ProcessThought(processArgs)
	if err != nil {
		t.Fatalf("Process phase failed: %v", err)
	}

	// 3. Verify gather phase is cleared
	gatherThoughtsAfter, _ := server.shm.GetPhaseThoughts("gather")
	if len(gatherThoughtsAfter) != 0 {
		t.Errorf("Expected gather thoughts to be cleared when entering process phase, but found %d", len(gatherThoughtsAfter))
	}

	// 4. Verify process phase files exist
	processThoughts, _ := server.shm.GetPhaseThoughts("process")
	if len(processThoughts) != 1 {
		t.Errorf("Expected 1 process thought, got %d", len(processThoughts))
	}
}

func TestMCPSessionConsistency(t *testing.T) {
	// This simulates exactly how main.go initializes the server once at boot
	server := NewDeepThinkingServer(0, 10, "", false)
	server.shm.ClearAll()
	server.thoughtHistory = nil

	workerCount := 3
	server.defaultWorkerCount = workerCount
	nextNeeded := true

	// Capture the Session ID from the first worker's response
	var sessionID string

	// Phase 1: Gather (3 Workers)
	for i := 1; i <= 3; i++ {
		resp, err := server.ProcessThought(ThoughtData{
			Thought:             fmt.Sprintf("Worker %d gather thought", i),
			Phase:               "gather",
			WorkerID:            i,
			ThinkingWorkerCount: workerCount,
			NextThoughtNeeded:   nextNeeded,
		})
		
		if err != nil {
			t.Fatalf("Failed on worker %d: %v", i, err)
		}

		if i == 1 {
			sessionID = resp.SessionID
		} else if resp.SessionID != sessionID {
			t.Fatalf("Session ID mismatch in gather phase! Expected %s, got %s", sessionID, resp.SessionID)
		}
	}

	// Phase 2: Process
	respProcess, err := server.ProcessThought(ThoughtData{
		Thought:             "Processing",
		Phase:               "process",
		WorkerID:            1,
		ThinkingWorkerCount: workerCount,
		NextThoughtNeeded:   nextNeeded,
	})

	if err != nil {
		t.Fatalf("Failed on process phase: %v", err)
	}

	if respProcess.SessionID != sessionID {
		t.Fatalf("Session ID mismatch in process phase! Expected %s, got %s", sessionID, respProcess.SessionID)
	}

	// Phase 3: Test
	respTest, err := server.ProcessThought(ThoughtData{
		Thought:             "Testing",
		Phase:               "test",
		WorkerID:            1,
		ThinkingWorkerCount: workerCount,
		NextThoughtNeeded:   false, // Finish the task to trigger cleanup
	})

	if err != nil {
		t.Fatalf("Failed on test phase: %v", err)
	}

	if respTest.SessionID != sessionID {
		t.Fatalf("Session ID mismatch in test phase! Expected %s, got %s", sessionID, respTest.SessionID)
	}
}

func TestContextAwareThinking(t *testing.T) {
	server := NewDeepThinkingServer(2, 10, "", false)
	defer os.RemoveAll(server.shm.shmPath)
	server.shm.ClearAll()
	server.thoughtHistory = nil

	track := "security"
	context1 := "Discovered tool: osvScanner"
	context2 := "Environment: Linux"

	// Worker 1
	resp1, err := server.ProcessThought(ThoughtData{
		Thought:             "Worker 1 analysis",
		Phase:               "gather",
		WorkerID:            1,
		ThinkingWorkerCount: 2,
		Track:               track,
		Context:             context1,
		NextThoughtNeeded:   true,
	})
	if err != nil {
		t.Fatalf("Worker 1 failed: %v", err)
	}
	if resp1.Track != track {
		t.Errorf("Expected track %s, got %s", track, resp1.Track)
	}
	if resp1.Context != context1 {
		t.Errorf("Expected context %s, got %s", context1, resp1.Context)
	}

	// Worker 2
	resp2, err := server.ProcessThought(ThoughtData{
		Thought:             "Worker 2 analysis",
		Phase:               "gather",
		WorkerID:            2,
		ThinkingWorkerCount: 2,
		Track:               track,
		Context:             context2,
		NextThoughtNeeded:   true,
	})
	if err != nil {
		t.Fatalf("Worker 2 failed: %v", err)
	}

	// Verify SuperIdea synthesis
	if resp2.Status != "all_workers_finished" {
		t.Errorf("Expected all_workers_finished, got %s", resp2.Status)
	}
	if !strings.Contains(strings.ToUpper(resp2.SuperIdea), "TRACK: SECURITY") {
		t.Error("SuperIdea missing track info")
	}
	if !strings.Contains(resp2.SuperIdea, context1) || !strings.Contains(resp2.SuperIdea, context2) {
		t.Error("SuperIdea missing combined context info")
	}
	if !strings.Contains(resp2.SuperIdea, "As this is a 'SECURITY' track") {
		t.Error("SuperIdea missing track-specific instructions")
	}
}

func TestValidateTrack(t *testing.T) {
	tests := []struct {
		track   string
		wantErr bool
	}{
		{"", false},
		{"bug-fix", false},
		{"feature_123", false},
		{"Security-Track", false},
		{"invalid track!", true},
		{"this_track_name_is_way_too_long_for_the_regex_to_allow", true},
	}

	for _, tt := range tests {
		err := validateTrack(tt.track)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateTrack(%q) error = %v, wantErr %v", tt.track, err, tt.wantErr)
		}
	}
}

func TestMarkdownFlowchartGeneration(t *testing.T) {
	server := NewDeepThinkingServer(1, 10, "", false)
	defer os.RemoveAll(server.shm.shmPath)
	server.shm.ClearAll()
	server.thoughtHistory = nil

	// 1. Gather
	server.ProcessThought(ThoughtData{
		Thought:           "Gather 1",
		Phase:             "gather",
		WorkerID:          1,
		NextThoughtNeeded: true,
	})

	// 2. Process
	server.ProcessThought(ThoughtData{
		Thought:           "Process 1",
		Phase:             "process",
		WorkerID:          1,
		NextThoughtNeeded: true,
	})

	// 3. Test with Diagram Request
	trueVal := true
	resp, err := server.ProcessThought(ThoughtData{
		Thought:           "Test 1",
		Phase:             "test",
		WorkerID:          1,
		GenerateDiagram:   &trueVal,
		NextThoughtNeeded: false,
	})

	if err != nil {
		t.Fatalf("ProcessThought failed: %v", err)
	}

	if resp.Flowchart == "" {
		t.Error("Expected Flowchart, got empty string")
	}

	expectedKeywords := []string{"```text", "DEEPTHINKING FLOWCHART", "--- GATHER PHASE ---", "--- PROCESS PHASE ---", "--- TEST PHASE ---", "T1 (W1)", "T2 (W1)", "T3 (W1)", "+----------------------------------------------------------+", "|"}
	for _, kw := range expectedKeywords {
		if !strings.Contains(resp.Flowchart, kw) {
			t.Errorf("Flowchart missing keyword: %s", kw)
		}
	}
}

func TestSecurityFeatures(t *testing.T) {
	server := NewDeepThinkingServer(1, 10, "", false)
	defer os.RemoveAll(server.shm.shmPath)
	server.shm.ClearAll()
	server.thoughtHistory = nil

	t.Run("Taint Propagation", func(t *testing.T) {
		// 1. Tainted thought
		resp, _ := server.ProcessThought(ThoughtData{
			Thought:           "Untrusted input",
			IsTainted:         true,
			NextThoughtNeeded: true,
		})
		if !resp.IsTainted {
			t.Error("Expected response to be tainted")
		}

		// 2. Subsequent thought should be tainted
		resp, _ = server.ProcessThought(ThoughtData{
			Thought:           "Derived thought",
			NextThoughtNeeded: true,
		})
		if !resp.IsTainted {
			t.Error("Expected subsequent response to be tainted")
		}
	})

	t.Run("Zero-Knowledge Synthesis", func(t *testing.T) {
		server.shm.ClearAll()
		server.thoughtHistory = nil

		// 1. Private thought
		server.ProcessThought(ThoughtData{
			Thought:             "Secret internal logic",
			Phase:               "gather",
			WorkerID:            1,
			IsPrivate:           true,
			ThinkingWorkerCount: 1,
			NextThoughtNeeded:   true,
		})

		// 2. Synthesis should redact it
		resp, _ := server.ProcessThought(ThoughtData{
			Thought:             "Final gather",
			Phase:               "gather",
			WorkerID:            1,
			ThinkingWorkerCount: 1,
			NextThoughtNeeded:   true,
		})

		if strings.Contains(resp.SuperIdea, "Secret internal logic") {
			t.Error("Private thought was not redacted from SuperIdea")
		}
		if !strings.Contains(resp.SuperIdea, "[PRIVATE THOUGHT - REDACTED FOR SECURITY]") {
			t.Error("Private thought placeholder missing from SuperIdea")
		}
	})

	t.Run("Dynamic Scaling Suggestion", func(t *testing.T) {
		server.shm.ClearAll()
		server.thoughtHistory = nil

		// Test baseline (short thought)
		resp, _ := server.ProcessThought(ThoughtData{
			Thought:           "Short thought.",
			ThoughtNumber:     1,
			NextThoughtNeeded: true,
		})
		if resp.SuggestedWorkerCount != 5 {
			t.Errorf("Expected SuggestedWorkerCount 5 for short task, got %d", resp.SuggestedWorkerCount)
		}

		// Test incremental scaling (700 chars)
		// Use non-hex characters to avoid redaction
		longThought := strings.Repeat("z", 700)
		resp, _ = server.ProcessThought(ThoughtData{
			Thought:           longThought,
			ThoughtNumber:     1,
			NextThoughtNeeded: true,
		})
		// 6 + (700-500)/200 = 7
		if resp.SuggestedWorkerCount != 7 {
			t.Errorf("Expected SuggestedWorkerCount 7 for 700 char task, got %d", resp.SuggestedWorkerCount)
		}

		// Test max scaling (1300 chars)
		veryLongThought := strings.Repeat("z", 1300)
		resp, _ = server.ProcessThought(ThoughtData{
			Thought:           veryLongThought,
			ThoughtNumber:     1,
			NextThoughtNeeded: true,
		})
		// 6 + (1300-500)/200 = 10
		if resp.SuggestedWorkerCount != 10 {
			t.Errorf("Expected SuggestedWorkerCount 10 for 1300 char task, got %d", resp.SuggestedWorkerCount)
		}
	})

	t.Run("Max Worker Limit Enforcement", func(t *testing.T) {
		// Create server with max 3 workers
		serverMax := NewDeepThinkingServer(1, 3, "", false)
		defer os.RemoveAll(serverMax.shm.shmPath)
		serverMax.shm.ClearAll()

		// 1. Test input capping
		resp, _ := serverMax.ProcessThought(ThoughtData{
			Thought:             "Test",
			ThinkingWorkerCount: 5, // Exceeds max
			NextThoughtNeeded:   true,
		})

		// We need to check the internal state or the status message
		// The status message for gather phase shows (1/ThinkingWorkerCount)
		if !strings.Contains(resp.Status, "(1/3)") {
			t.Errorf("Expected status to show cap at 3 workers, got %s", resp.Status)
		}

		// 2. Test suggestion capping
		resp2, _ := serverMax.ProcessThought(ThoughtData{
			Thought:           "I need to refactor the entire system architecture to improve performance.",
			ThoughtNumber:     1,
			NextThoughtNeeded: true,
		})

		if resp2.SuggestedWorkerCount > 3 {
			t.Errorf("Expected SuggestedWorkerCount to be capped at 3, got %d", resp2.SuggestedWorkerCount)
		}
	})
}

func TestFlowchartFileSaving(t *testing.T) {
	server := NewDeepThinkingServer(1, 10, "", true) // Enable diagram by default
	defer os.RemoveAll(server.shm.shmPath)
	server.shm.ClearAll()
	server.thoughtHistory = nil

	// Clean up any existing test files
	files, _ := filepath.Glob("deepthinking-*-flow.md")
	for _, f := range files {
		os.Remove(f)
	}
	os.Remove("deepthinking-flow.md")

	// 1. First session/thought should create deepthinking-flow.md
	server.ProcessThought(ThoughtData{
		Thought:           "Thought 1",
		Phase:             "gather",
		WorkerID:          1,
		NextThoughtNeeded: true,
	})

	if _, err := os.Stat("deepthinking-flow.md"); os.IsNotExist(err) {
		t.Error("Expected deepthinking-flow.md to be created")
	}
	os.Remove("deepthinking-flow.md")

	// 2. Create a dummy file to force increment
	os.WriteFile("deepthinking-flow.md", []byte("dummy"), 0644)
	defer os.Remove("deepthinking-flow.md")

	server2 := NewDeepThinkingServer(1, 10, "", true)
	server2.ProcessThought(ThoughtData{
		Thought:           "Thought 2",
		Phase:             "gather",
		WorkerID:          1,
		NextThoughtNeeded: true,
	})

	if _, err := os.Stat("deepthinking-1-flow.md"); os.IsNotExist(err) {
		t.Error("Expected deepthinking-1-flow.md to be created")
	}
	os.Remove("deepthinking-1-flow.md")

	// 3. Create another dummy to force deepthinking-2-flow.md
	os.WriteFile("deepthinking-1-flow.md", []byte("dummy"), 0644)
	defer os.Remove("deepthinking-1-flow.md")

	server3 := NewDeepThinkingServer(1, 10, "", true)
	server3.ProcessThought(ThoughtData{
		Thought:           "Thought 3",
		Phase:             "gather",
		WorkerID:          1,
		NextThoughtNeeded: true,
	})

	if _, err := os.Stat("deepthinking-2-flow.md"); os.IsNotExist(err) {
		t.Error("Expected deepthinking-2-flow.md to be created")
	}
	os.Remove("deepthinking-2-flow.md")
}

func TestRedaction(t *testing.T) {
	server := NewDeepThinkingServer(0, 10, "", false)
	defer os.RemoveAll(server.shm.shmPath)

	t.Run("Redact Thought and Context", func(t *testing.T) {
		secretEmail := "user@example.com"
		secretKey := "sk-123456789012345678901234567890"
		
		args := ThoughtData{
			Thought:           fmt.Sprintf("My email is %s", secretEmail),
			Context:           fmt.Sprintf("Found key: %s", secretKey),
			NextThoughtNeeded: false,
		}

		resp, err := server.ProcessThought(args)
		if err != nil {
			t.Fatalf("ProcessThought failed: %v", err)
		}

		if strings.Contains(resp.Context, secretKey) {
			t.Error("Context was not redacted")
		}
		if !strings.Contains(resp.Context, "[REDACTED OPENAI KEY]") {
			t.Error("Context redaction placeholder missing")
		}

		// Also check history (which is what gets saved to SHM)
		lastThought := server.thoughtHistory[len(server.thoughtHistory)-1]
		if strings.Contains(lastThought.Thought, secretEmail) {
			t.Error("Thought in history was not redacted")
		}
		if strings.Contains(lastThought.Context, secretKey) {
			t.Error("Context in history was not redacted")
		}
	})
}

func TestSuperIdeaSynthesisHint(t *testing.T) {
	server := NewDeepThinkingServer(1, 10, "", false)
	defer os.RemoveAll(server.shm.shmPath)
	server.shm.ClearAll()
	server.thoughtHistory = nil

	// Submit a gather thought to trigger synthesis (worker count is 1)
	resp, err := server.ProcessThought(ThoughtData{
		Thought:             "Gathering info",
		Phase:               "gather",
		WorkerID:            1,
		ThinkingWorkerCount: 1,
		NextThoughtNeeded:   true,
	})

	if err != nil {
		t.Fatalf("ProcessThought failed: %v", err)
	}

	if !strings.Contains(resp.SuperIdea, "💡 TIP: If you want to visualize this thinking path") {
		t.Error("SuperIdea missing the diagram visualization tip")
	}
}
