package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var ShmRoot = getEnv("SHM_ROOT", "/dev/shm/deepthinking-ng")

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

type SharedMemoryManager struct {
	mu sync.Mutex
}

func NewSharedMemoryManager() *SharedMemoryManager {
	// Ensure root directory exists with restrictive permissions (0700)
	os.MkdirAll(ShmRoot, 0700)
	return &SharedMemoryManager{}
}

func (m *SharedMemoryManager) SaveThought(phase string, workerID int, data ThoughtData) error {
	// Validate phase to prevent path traversal
	phase = filepath.Base(filepath.Clean(phase))
	if phase == "." || phase == ".." || phase == "/" {
		return fmt.Errorf("invalid phase name")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	dir := filepath.Join(ShmRoot, phase)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create shm dir: %w", err)
	}

	filename := fmt.Sprintf("worker_%d.json", workerID)
	path := filepath.Join(dir, filename)

	bytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal thought: %w", err)
	}

	// Use restrictive permissions (0600)
	if err := os.WriteFile(path, bytes, 0600); err != nil {
		return fmt.Errorf("failed to write to shm: %w", err)
	}

	return nil
}

func (m *SharedMemoryManager) GetPhaseThoughts(phase string) ([]ThoughtData, error) {
	// Validate phase to prevent path traversal
	phase = filepath.Base(filepath.Clean(phase))
	if phase == "." || phase == ".." || phase == "/" {
		return nil, fmt.Errorf("invalid phase name")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	dir := filepath.Join(ShmRoot, phase)
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ThoughtData{}, nil
		}
		return nil, fmt.Errorf("failed to read shm dir: %w", err)
	}

	var thoughts []ThoughtData
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			path := filepath.Join(dir, file.Name())
			bytes, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			var data ThoughtData
			if err := json.Unmarshal(bytes, &data); err == nil {
				thoughts = append(thoughts, data)
			}
		}
	}

	return thoughts, nil
}

func (m *SharedMemoryManager) ClearAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return os.RemoveAll(ShmRoot)
}
