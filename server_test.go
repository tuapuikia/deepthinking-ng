package main

import (
	"testing"
)

func TestSequentialThinking(t *testing.T) {
	server := NewSequentialThinkingServer()
	// Ensure we start fresh
	server.shm.ClearAll()

	t.Run("Basic Thought", func(t *testing.T) {
		thought := "Initial analysis"
		nextNeeded := true
		args := ThoughtData{
			Thought:           thought,
			NextThoughtNeeded: &nextNeeded,
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
			NextThoughtNeeded: &nextNeeded,
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
				NextThoughtNeeded:   &nextNeeded,
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
				NextThoughtNeeded:   &nextNeeded,
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

	t.Run("Completion and Cleanup", func(t *testing.T) {
		nextNeeded := false
		args := ThoughtData{
			Thought:           "Final conclusion",
			NextThoughtNeeded: &nextNeeded,
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
	nextNeeded := true
	server.ProcessThought(ThoughtData{Thought: "Test", NextThoughtNeeded: &nextNeeded})

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
