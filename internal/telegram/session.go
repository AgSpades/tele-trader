// Package telegram provides the Telegram userbot integration using gotd/td.
// It handles session persistence, connection management, and message dispatching.
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// FileSessionStorage implements tg.SessionStorage backed by a local JSON file.
// It is safe for concurrent use.
type FileSessionStorage struct {
	mu   sync.RWMutex
	path string
}

// NewFileSessionStorage returns a FileSessionStorage that persists to path.
func NewFileSessionStorage(path string) *FileSessionStorage {
	return &FileSessionStorage{path: path}
}

// sessionData is the on-disk format.
type sessionData struct {
	Data []byte `json:"data"`
}

// LoadSession reads session bytes from disk.
// Returns nil, nil if the file does not exist yet (first run).
func (s *FileSessionStorage) LoadSession(_ context.Context) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	raw, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session: read file %q: %w", s.path, err)
	}

	var sd sessionData
	if err := json.Unmarshal(raw, &sd); err != nil {
		return nil, fmt.Errorf("session: unmarshal %q: %w", s.path, err)
	}
	return sd.Data, nil
}

// StoreSession writes session bytes to disk atomically via a temp file.
func (s *FileSessionStorage) StoreSession(_ context.Context, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sd := sessionData{Data: data}
	raw, err := json.Marshal(sd)
	if err != nil {
		return fmt.Errorf("session: marshal: %w", err)
	}

	// Write to a temp file then rename for atomicity.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return fmt.Errorf("session: write temp file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("session: rename temp file: %w", err)
	}
	return nil
}
