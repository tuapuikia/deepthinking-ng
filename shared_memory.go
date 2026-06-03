package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getDefaultShmRoot() string {
	switch runtime.GOOS {
	case "linux":
		return "/dev/shm/deepthinking-ng"
	case "darwin":
		return "/tmp/deepthinking-ng"
	case "windows":
		return filepath.Join(os.TempDir(), "deepthinking-ng")
	default:
		return filepath.Join(os.TempDir(), "deepthinking-ng")
	}
}

func isPathSafe(path string) bool {
	path = filepath.Clean(path)
	switch runtime.GOOS {
	case "linux":
		// Stricter check: must start with /dev/shm/ to prevent /dev/shm-evil
		return strings.HasPrefix(path, "/dev/shm/")
	case "darwin":
		// On macOS, /tmp is a symlink to /private/tmp, but we usually use /tmp
		return strings.HasPrefix(path, "/tmp/") || strings.HasPrefix(path, "/private/tmp/") || strings.HasPrefix(path, os.TempDir())
	case "windows":
		return strings.HasPrefix(strings.ToLower(path), strings.ToLower(os.TempDir()))
	default:
		return strings.HasPrefix(path, os.TempDir())
	}
}

func generateSessionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp + pid if randomness fails
		return fmt.Sprintf("fallback_%d_%d", os.Getpid(), os.Getppid())
	}
	return hex.EncodeToString(bytes)
}

type SharedMemoryManager struct {
	mu        sync.Mutex
	sessionID string
	shmPath   string
	aead      cipher.AEAD
}

func NewSharedMemoryManager(shmRoot string) *SharedMemoryManager {
	sessionID := getEnv("GEMINI_SESSION_ID", "")
	if sessionID == "" {
		sessionID = generateSessionID()
	} else {
		// Sanitize user-provided session ID to prevent path traversal
		sessionID = filepath.Base(filepath.Clean(sessionID))
		if sessionID == "." || sessionID == ".." || sessionID == "/" {
			sessionID = generateSessionID()
		}
	}

	defaultRoot := getDefaultShmRoot()
	if shmRoot == "" {
		shmRoot = getEnv("SHM_ROOT", defaultRoot)
	}
	shmRoot = filepath.Clean(shmRoot)

	// Strict Rule: Only accept paths in volatile/temp areas to prevent path traversal or dangerous path usage
	if !isPathSafe(shmRoot) {
		fmt.Fprintf(os.Stderr, "Warning: shm-root '%s' is outside volatile storage for %s. Falling back to default: %s\n", shmRoot, runtime.GOOS, defaultRoot)
		shmRoot = defaultRoot
	}

	shmPath := filepath.Join(shmRoot, sessionID)

	// Clean up stale session directories in shmRoot during startup asynchronously
	go func() {
		if entries, err := os.ReadDir(shmRoot); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					dirPath := filepath.Join(shmRoot, entry.Name())
					// Don't delete the current session directory we just created or are about to use
					if entry.Name() == sessionID {
						continue
					}
					// Check modification time
					if info, err := os.Stat(dirPath); err == nil {
						// If not modified for more than 2 hours, clean it up
						if time.Since(info.ModTime()) > 2*time.Hour {
							fmt.Fprintf(os.Stderr, "Cleaning up stale shared memory session: %s (inactive since %v)\n", entry.Name(), info.ModTime())
							os.RemoveAll(dirPath)
						}
					}
				}
			}
		}
	}()

	// Ensure root directory exists with restrictive permissions (0700)
	// Note: On Windows, permissions are handled differently, but 0700 is a safe default for Unix-like systems.
	os.MkdirAll(shmPath, 0700)
	fmt.Fprintf(os.Stderr, "Shared memory initialized with random session ID: %s\n", sessionID)
	fmt.Fprintf(os.Stderr, "Path: %s\n", shmPath)

	// Initialize encryption (AES-256-GCM)
	// Go's crypto/aes automatically uses AES-NI if available.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: failed to generate encryption key: %v\n", err)
		os.Exit(1)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: failed to create AES cipher: %v\n", err)
		os.Exit(1)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: failed to create GCM: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Shared memory encryption initialized: AES-256-GCM (BoringCrypto enabled)\n")

	return &SharedMemoryManager{
		sessionID: sessionID,
		shmPath:   shmPath,
		aead:      aead,
	}
}

func (m *SharedMemoryManager) encrypt(data []byte) ([]byte, error) {
	nonce := make([]byte, m.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Seal appends the ciphertext and tag to the nonce
	return m.aead.Seal(nonce, nonce, data, nil), nil
}

func (m *SharedMemoryManager) decrypt(data []byte) ([]byte, error) {
	ns := m.aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:ns], data[ns:]
	return m.aead.Open(nil, nonce, ciphertext, nil)
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

	// Encrypt the data before writing to shm
	encrypted, err := m.encrypt(bytes)
	if err != nil {
		return fmt.Errorf("failed to encrypt thought: %w", err)
	}

	// Use restrictive permissions (0600)
	if err := os.WriteFile(path, encrypted, 0600); err != nil {
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

			// Decrypt the data
			decrypted, err := m.decrypt(bytes)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to decrypt shm file %s: %v (skipping)\n", file.Name(), err)
				continue
			}

			var data ThoughtData
			if err := json.Unmarshal(decrypted, &data); err == nil {
				thoughts = append(thoughts, data)
			}
		}
	}

	return thoughts, nil
}

func (m *SharedMemoryManager) ClearAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Use rename-and-delete pattern for near-instant response
	tempPath := m.shmPath + ".tmp." + generateSessionID()[:8]
	
	// Rename current session dir to temp path
	if err := os.Rename(m.shmPath, tempPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		// If rename fails, fallback to synchronous delete to be safe
		return os.RemoveAll(m.shmPath)
	}

	// Immediately recreate the empty session directory
	if err := os.MkdirAll(m.shmPath, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to recreate shm path after rename: %v\n", err)
	}

	// Delete the old contents in the background
	go func() {
		if err := os.RemoveAll(tempPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: background cleanup of %s failed: %v\n", tempPath, err)
		}
	}()

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
	
	// Use rename-and-delete pattern for near-instant response
	tempPath := dir + ".tmp." + generateSessionID()[:8]
	
	if err := os.Rename(dir, tempPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		// Fallback to synchronous delete
		return os.RemoveAll(dir)
	}

	// Delete in background
	go func() {
		if err := os.RemoveAll(tempPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: background cleanup of phase %s failed: %v\n", phase, err)
		}
	}()

	return nil
}
