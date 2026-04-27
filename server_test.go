package main

import (
	"fmt"

	"os"

	"testing"
)

func TestSequentialThinking(t *testing.T) {
	server := NewSequentialThinkingServer()
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
			t.Error("Expected error when entering 'process' phase before 'gather' is complete")
		} else if err.Error() != "STRICT WORKFLOW VIOLATION: Cannot enter 'process' phase. 'gather' phase is incomplete. You must gather perspectives from 3 workers (currently have 0). Use phase='gather' with different workerIds" {
			t.Errorf("Unexpected error message: %v", err)
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
			t.Error("Expected error when entering 'test' phase before 'process' is complete")
		} else if err.Error() != "STRICT WORKFLOW VIOLATION: Cannot enter 'test' phase. 'process' phase is incomplete. You must process the gathered ideas first using phase='process'" {
			t.Errorf("Unexpected error message: %v", err)
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
	server := NewSequentialThinkingServer()
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
	server := NewSequentialThinkingServer()
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
	server := NewSequentialThinkingServer()
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
	server := NewSequentialThinkingServer()
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
