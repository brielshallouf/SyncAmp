// Package store persists backup run history as a small JSON file.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/brielshallouf/syncamp/internal/platform"
)

// Run is the record of one backup execution.
type Run struct {
	Profile  string    `json:"profile"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Status   string    `json:"status"` // success | warning | failed | timeout
	ExitCode int       `json:"exitCode"`
	Trigger  string    `json:"trigger"` // schedule | manual
	Error    string    `json:"error,omitempty"`
	Tail     []string  `json:"tail,omitempty"` // last lines of client output
}

// Store is a concurrency-safe, size-capped run history.
type Store struct {
	mu    sync.Mutex
	path  string
	limit int
	runs  []Run
}

// Open loads (or creates) the history file.
func Open(path string, limit int) (*Store, error) {
	s := &Store{path: path, limit: limit}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.runs); err != nil {
		// Corrupt history must never stop backups: start fresh, keep the bad file.
		_ = os.Rename(path, path+".corrupt")
		s.runs = nil
	}
	return s, nil
}

// Add appends a run and persists.
func (s *Store) Add(r Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = append(s.runs, r)
	if s.limit > 0 && len(s.runs) > s.limit {
		s.runs = s.runs[len(s.runs)-s.limit:]
	}
	if err := platform.SecureDir(filepath.Dir(s.path)); err != nil {
		return err
	}
	b, _ := json.Marshal(s.runs)
	return platform.WriteSecret(s.path, b)
}

// All returns a copy, oldest first.
func (s *Store) All() []Run {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Run(nil), s.runs...)
}

// Last returns the most recent run for a profile.
func (s *Store) Last(profile string) (Run, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.runs) - 1; i >= 0; i-- {
		if s.runs[i].Profile == profile {
			return s.runs[i], true
		}
	}
	return Run{}, false
}
