package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// State persists between iterations and across crashes.
type State struct {
	Mode         string        `json:"mode"`
	TaskID       string        `json:"task_id"`
	Attempt      int           `json:"attempt"`
	MaxAttempts  int           `json:"max_attempts"`
	GapDetails   string        `json:"gap_details"`
	LastOutcome  string        `json:"last_outcome"`
	LastOutcomes []string      `json:"last_outcomes"`
	BlockedTasks []BlockedTask `json:"blocked_tasks"`
	Iteration    int           `json:"iteration"`
}

// BlockedTask records a task that was blocked during execution.
type BlockedTask struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
	Gaps   string `json:"gaps,omitempty"`
}

// LoadState reads state from a JSON file. Returns a fresh state if the file
// does not exist.
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{
				Mode:        "normal",
				MaxAttempts: 3,
			}, nil
		}
		return nil, err
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.MaxAttempts == 0 {
		s.MaxAttempts = 3
	}
	if s.Mode == "" {
		s.Mode = "normal"
	}
	return &s, nil
}

// Save writes the state atomically (temp file + rename).
func (s *State) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// Reset clears task-specific fields, returning to normal mode.
func (s *State) Reset() {
	s.Mode = "normal"
	s.TaskID = ""
	s.Attempt = 0
	s.GapDetails = ""
	s.LastOutcome = ""
}

// RecordOutcome appends an outcome and keeps the last 20 entries.
func (s *State) RecordOutcome(outcome string) {
	s.LastOutcome = outcome
	s.LastOutcomes = append(s.LastOutcomes, outcome)
	if len(s.LastOutcomes) > 20 {
		s.LastOutcomes = s.LastOutcomes[len(s.LastOutcomes)-20:]
	}
}

// IsStuck returns true when the two most recent outcomes are both NO_OP.
func (s *State) IsStuck() bool {
	n := len(s.LastOutcomes)
	if n < 2 {
		return false
	}
	return s.LastOutcomes[n-1] == OutcomeNoOp && s.LastOutcomes[n-2] == OutcomeNoOp
}
