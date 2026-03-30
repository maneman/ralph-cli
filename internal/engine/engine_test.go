package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mane/ralph-cli/internal/agent"
)

func TestLoadStateSaveRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Loading a non-existent file returns a fresh state.
	s, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.Mode != "normal" {
		t.Errorf("mode = %q, want %q", s.Mode, "normal")
	}
	if s.MaxAttempts != 3 {
		t.Errorf("max_attempts = %d, want 3", s.MaxAttempts)
	}

	// Mutate and persist.
	s.TaskID = "task-42"
	s.Attempt = 2
	s.Mode = "gap-fill"
	s.GapDetails = "missing validation"
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload and verify.
	s2, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState after save: %v", err)
	}
	if s2.TaskID != "task-42" {
		t.Errorf("task_id = %q, want %q", s2.TaskID, "task-42")
	}
	if s2.Attempt != 2 {
		t.Errorf("attempt = %d, want 2", s2.Attempt)
	}
	if s2.Mode != "gap-fill" {
		t.Errorf("mode = %q, want %q", s2.Mode, "gap-fill")
	}
	if s2.GapDetails != "missing validation" {
		t.Errorf("gap_details = %q, want %q", s2.GapDetails, "missing validation")
	}
}

func TestStateReset(t *testing.T) {
	s := &State{
		Mode:        "gap-fill",
		TaskID:      "task-1",
		Attempt:     2,
		GapDetails:  "some gaps",
		LastOutcome: "NO_OP",
		MaxAttempts: 3,
	}
	s.Reset()

	if s.Mode != "normal" {
		t.Errorf("mode = %q, want %q", s.Mode, "normal")
	}
	if s.TaskID != "" {
		t.Errorf("task_id = %q, want empty", s.TaskID)
	}
	if s.Attempt != 0 {
		t.Errorf("attempt = %d, want 0", s.Attempt)
	}
	if s.GapDetails != "" {
		t.Errorf("gap_details = %q, want empty", s.GapDetails)
	}
	if s.LastOutcome != "" {
		t.Errorf("last_outcome = %q, want empty", s.LastOutcome)
	}
}

func TestStateIsStuck(t *testing.T) {
	tests := []struct {
		name     string
		outcomes []string
		want     bool
	}{
		{"empty", nil, false},
		{"single NO_OP", []string{"NO_OP"}, false},
		{"two consecutive NO_OPs", []string{"NO_OP", "NO_OP"}, true},
		{"NO_OP-SUCCESS-NO_OP", []string{"NO_OP", "SUCCESS", "NO_OP"}, false},
		{"SUCCESS then two NO_OPs", []string{"SUCCESS", "NO_OP", "NO_OP"}, true},
		{"three NO_OPs", []string{"NO_OP", "NO_OP", "NO_OP"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &State{LastOutcomes: tt.outcomes}
			if got := s.IsStuck(); got != tt.want {
				t.Errorf("IsStuck() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStateRecordOutcomeRollingWindow(t *testing.T) {
	s := &State{Mode: "normal", MaxAttempts: 3}
	for i := 0; i < 25; i++ {
		s.RecordOutcome("SUCCESS")
	}
	if len(s.LastOutcomes) != 20 {
		t.Errorf("len(LastOutcomes) = %d, want 20", len(s.LastOutcomes))
	}
	if s.LastOutcome != "SUCCESS" {
		t.Errorf("LastOutcome = %q, want %q", s.LastOutcome, "SUCCESS")
	}
}

func TestClassifyOutcome(t *testing.T) {
	tests := []struct {
		name        string
		result      *agent.RunResult
		commitCount int
		state       *State
		err         error
		want        string
	}{
		{
			name:  "error returns AGENT_CRASH",
			err:   os.ErrClosed,
			state: &State{Mode: "normal"},
			want:  OutcomeAgentCrash,
		},
		{
			name:  "nil result returns AGENT_CRASH",
			state: &State{Mode: "normal"},
			want:  OutcomeAgentCrash,
		},
		{
			name:        "success with task_id",
			result:      &agent.RunResult{TaskID: "t1", Outcome: "success"},
			commitCount: 1,
			state:       &State{Mode: "normal"},
			want:        OutcomeSuccess,
		},
		{
			name:   "stuck with IsStuck true",
			result: &agent.RunResult{Outcome: "stuck"},
			state:  &State{LastOutcomes: []string{"NO_OP", "NO_OP"}},
			want:   OutcomeStuck,
		},
		{
			name:   "stuck without IsStuck",
			result: &agent.RunResult{Outcome: "stuck"},
			state:  &State{LastOutcomes: []string{"SUCCESS"}},
			want:   OutcomeNoOp,
		},
		{
			name:   "blocked returns STUCK",
			result: &agent.RunResult{Outcome: "blocked"},
			state:  &State{Mode: "normal"},
			want:   OutcomeStuck,
		},
		{
			name:   "zero commits returns NO_OP",
			result: &agent.RunResult{Outcome: ""},
			state:  &State{Mode: "normal"},
			want:   OutcomeNoOp,
		},
		{
			name:        "commits with unknown outcome",
			result:      &agent.RunResult{Outcome: "unknown"},
			commitCount: 3,
			state:       &State{Mode: "normal"},
			want:        OutcomeSuccess,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyOutcome(tt.result, tt.commitCount, tt.state, tt.err)
			if got != tt.want {
				t.Errorf("ClassifyOutcome() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAcquireReleaseLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "ralph.lock")

	if err := AcquireLock(lockPath); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	// Lock file should exist.
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file should exist: %v", err)
	}

	// A second acquire with the same (running) PID must fail.
	if err := AcquireLock(lockPath); err == nil {
		t.Error("expected error on double acquire")
	}

	if err := ReleaseLock(lockPath); err != nil {
		t.Fatalf("ReleaseLock: %v", err)
	}

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should not exist after release")
	}
}

func TestIsStaleWithNonExistentPID(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "ralph.lock")

	// Write a PID that almost certainly does not exist.
	if err := os.WriteFile(lockPath, []byte("9999999"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if !isStale(lockPath) {
		t.Error("expected stale for non-existent PID")
	}

	// AcquireLock should succeed after detecting a stale lock.
	if err := AcquireLock(lockPath); err != nil {
		t.Fatalf("AcquireLock after stale: %v", err)
	}
	_ = ReleaseLock(lockPath)
}
