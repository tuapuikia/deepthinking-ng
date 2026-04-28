package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func generateSessionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp if randomness fails
		return fmt.Sprintf("fallback_%d", os.Getpid())
	}
	return hex.EncodeToString(bytes)
}

type SharedMemoryManager struct {
	mu        sync.Mutex
	sessionID string
	shmPath   string
}

func NewSharedMemoryManager(shmRoot string) *SharedMemoryManager {
	sessionID := getEnv("GEMINI_SESSION_ID", "")
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	if shmRoot == "" {
		shmRoot = getEnv("SHM_ROOT", "/dev/shm/deepthinking-ng")
	}

	shmPath := filepath.Join(shmRoot, sessionID)

	// Ensure root directory exists with restrictive permissions (0700)
	os.MkdirAll(shmPath, 0700)
	fmt.Fprintf(os.Stderr, "Shared memory initialized with random session ID: %s\n", sessionID)
	fmt.Fprintf(os.Stderr, "Path: %s\n", shmPath)

	return &SharedMemoryManager{
		sessionID: sessionID,
		shmPath:   shmPath,
	}
}

func (m *SharedMemoryManager) GetSessionID() string {
	return m.sessionID
}

func (m *SharedMemoryManager) SaveThought(phase string, workerID int, data ThoughtData) error {
	// Validate phase to prevent path traversal
	phase = filepath.Base(filepath.Clean(phase))
	if phase == "." || phase == ".." || phase == "/" {
		return fmt.Errorf("invalid phase name")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	dir := filepath.Join(m.shmPath, phase)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create shm dir: %w", err)
	}

	hashBytes := sha256.Sum256([]byte(data.Thought))
	hashStr := hex.EncodeToString(hashBytes[:])[:8]
	filename := fmt.Sprintf("worker_%d_%s_%d.json", workerID, hashStr, data.ThoughtNumber)
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

	dir := filepath.Join(m.shmPath, phase)
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

	// Read the directory contents
	entries, err := os.ReadDir(m.shmPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Remove each entry (gather, process, test folders)
	for _, entry := range entries {
		err := os.RemoveAll(filepath.Join(m.shmPath, entry.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %v\n", entry.Name(), err)
		}
	}
	return nil
}

func (m *SharedMemoryManager) ClearPhase(phase string) error {
	phase = filepath.Base(filepath.Clean(phase))
	if phase == "." || phase == ".." || phase == "/" {
		return fmt.Errorf("invalid phase name")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	dir := filepath.Join(m.shmPath, phase)
	return os.RemoveAll(dir)
}
