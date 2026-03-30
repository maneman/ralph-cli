package telemetry

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogOutcome_WritesValidJSONL(t *testing.T) {
	dir := t.TempDir()
	lg, err := NewLogger(dir, 7)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer lg.Close()

	outcomes := []IterationOutcome{
		{Iteration: 1, TaskID: "task-a", Outcome: "SUCCESS", Duration: 2 * time.Second, Attempt: 1, Commits: 1, Timestamp: time.Now()},
		{Iteration: 2, TaskID: "task-b", Outcome: "GAPS_FOUND", Duration: 3 * time.Second, Attempt: 2, Commits: 0, GapCount: 3, Timestamp: time.Now()},
	}

	for _, o := range outcomes {
		if err := lg.LogOutcome(o); err != nil {
			t.Fatalf("LogOutcome: %v", err)
		}
	}

	// Close to flush, then read back.
	lg.Close()

	f, err := os.Open(filepath.Join(dir, outcomesFileName))
	if err != nil {
		t.Fatalf("open outcomes: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var decoded []IterationOutcome
	for scanner.Scan() {
		var o IterationOutcome
		if err := json.Unmarshal(scanner.Bytes(), &o); err != nil {
			t.Fatalf("unmarshal line: %v", err)
		}
		decoded = append(decoded, o)
	}

	if len(decoded) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(decoded))
	}
	if decoded[0].Outcome != "SUCCESS" {
		t.Errorf("expected SUCCESS, got %s", decoded[0].Outcome)
	}
	if decoded[1].GapCount != 3 {
		t.Errorf("expected GapCount 3, got %d", decoded[1].GapCount)
	}
}

func TestLogRun_AppendsToRunsFile(t *testing.T) {
	dir := t.TempDir()
	lg, err := NewLogger(dir, 7)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer lg.Close()

	s1 := RunSummary{Iterations: 5, Successes: 4, SuccessRate: 0.8}
	s2 := RunSummary{Iterations: 3, Successes: 3, SuccessRate: 1.0}

	if err := lg.LogRun(s1); err != nil {
		t.Fatalf("LogRun s1: %v", err)
	}
	if err := lg.LogRun(s2); err != nil {
		t.Fatalf("LogRun s2: %v", err)
	}

	runs, err := readRunSummaries(filepath.Join(dir, runsFileName))
	if err != nil {
		t.Fatalf("readRunSummaries: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	if runs[0].SuccessRate != 0.8 {
		t.Errorf("expected 0.8, got %f", runs[0].SuccessRate)
	}
	if runs[1].SuccessRate != 1.0 {
		t.Errorf("expected 1.0, got %f", runs[1].SuccessRate)
	}
}

func TestCleanOldLogs(t *testing.T) {
	dir := t.TempDir()
	lg, err := NewLogger(dir, 7)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer lg.Close()

	// Create an "old" log file with a past modification time.
	oldLog := filepath.Join(dir, "ralph-20240101-120000-iteration-1.log")
	if err := os.WriteFile(oldLog, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(oldLog, past, past); err != nil {
		t.Fatal(err)
	}

	// Create a "new" log file.
	newLog := filepath.Join(dir, "ralph-20250101-120000-iteration-2.log")
	if err := os.WriteFile(newLog, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := lg.CleanOldLogs(); err != nil {
		t.Fatalf("CleanOldLogs: %v", err)
	}

	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Error("old log should have been removed")
	}
	if _, err := os.Stat(newLog); err != nil {
		t.Error("new log should still exist")
	}
}

func TestLoadBaseline_ComputesCorrectAverages(t *testing.T) {
	dir := t.TempDir()
	runsFile := filepath.Join(dir, runsFileName)

	lg, err := NewLogger(dir, 7)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	runs := []RunSummary{
		{SuccessRate: 0.6, GapRate: 0.2, TotalTime: 10 * time.Second, AvgRetries: 1.0},
		{SuccessRate: 0.8, GapRate: 0.1, TotalTime: 20 * time.Second, AvgRetries: 2.0},
		{SuccessRate: 1.0, GapRate: 0.0, TotalTime: 30 * time.Second, AvgRetries: 3.0},
	}
	for _, r := range runs {
		if err := lg.LogRun(r); err != nil {
			t.Fatalf("LogRun: %v", err)
		}
	}
	lg.Close()

	bl, err := LoadBaseline(runsFile, 5)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	if bl == nil {
		t.Fatal("expected non-nil baseline")
	}

	expectedSR := (0.6 + 0.8 + 1.0) / 3
	if math.Abs(bl.SuccessRate.Baseline-expectedSR) > 1e-9 {
		t.Errorf("SuccessRate baseline: want %f, got %f", expectedSR, bl.SuccessRate.Baseline)
	}

	expectedGR := (0.2 + 0.1 + 0.0) / 3
	if math.Abs(bl.GapRate.Baseline-expectedGR) > 1e-9 {
		t.Errorf("GapRate baseline: want %f, got %f", expectedGR, bl.GapRate.Baseline)
	}
}

func TestLoadBaseline_MissingFileReturnsNil(t *testing.T) {
	bl, err := LoadBaseline("/nonexistent/path/runs.jsonl", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bl != nil {
		t.Error("expected nil baseline for missing file")
	}
}

func TestComputeBaseline_GeneratesCorrectDeltas(t *testing.T) {
	dir := t.TempDir()
	runsFile := filepath.Join(dir, runsFileName)

	lg, err := NewLogger(dir, 7)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	// Write 3 historical runs with identical stats.
	for i := 0; i < 3; i++ {
		if err := lg.LogRun(RunSummary{
			SuccessRate: 0.5,
			GapRate:     0.3,
			TotalTime:   10 * time.Second,
			AvgRetries:  2.0,
		}); err != nil {
			t.Fatalf("LogRun: %v", err)
		}
	}
	lg.Close()

	current := RunSummary{
		SuccessRate: 0.8,  // improved
		GapRate:     0.1,  // improved (lower)
		TotalTime:   5 * time.Second, // improved (lower)
		AvgRetries:  3.0,  // regressed (higher)
	}

	bl := ComputeBaseline(current, runsFile)
	if bl == nil {
		t.Fatal("expected non-nil baseline")
	}

	// Success rate: higher is better, 0.8 > 0.5 → ▲
	if bl.SuccessRate.Symbol != "▲" {
		t.Errorf("SuccessRate symbol: want ▲, got %s", bl.SuccessRate.Symbol)
	}
	if math.Abs(bl.SuccessRate.Diff-0.3) > 1e-9 {
		t.Errorf("SuccessRate diff: want 0.3, got %f", bl.SuccessRate.Diff)
	}

	// Gap rate: lower is better, 0.1 < 0.3 → ▲
	if bl.GapRate.Symbol != "▲" {
		t.Errorf("GapRate symbol: want ▲, got %s", bl.GapRate.Symbol)
	}

	// Duration: lower is better, 5s < 10s → ▲
	if bl.AvgDuration.Symbol != "▲" {
		t.Errorf("AvgDuration symbol: want ▲, got %s", bl.AvgDuration.Symbol)
	}

	// Retries: lower is better, 3.0 > 2.0 → ▼
	if bl.AvgRetries.Symbol != "▼" {
		t.Errorf("AvgRetries symbol: want ▼, got %s", bl.AvgRetries.Symbol)
	}
}

func TestDeltaSymbols(t *testing.T) {
	tests := []struct {
		name           string
		current        float64
		baseline       float64
		higherIsBetter bool
		wantSymbol     string
	}{
		{"improvement higher-is-better", 0.9, 0.5, true, "▲"},
		{"regression higher-is-better", 0.3, 0.5, true, "▼"},
		{"no change", 0.5, 0.5, true, "─"},
		{"improvement lower-is-better", 0.2, 0.5, false, "▲"},
		{"regression lower-is-better", 0.8, 0.5, false, "▼"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := computeDelta(tt.current, tt.baseline, tt.higherIsBetter)
			if d.Symbol != tt.wantSymbol {
				t.Errorf("symbol: want %s, got %s", tt.wantSymbol, d.Symbol)
			}
			expectedDiff := tt.current - tt.baseline
			if math.Abs(d.Diff-expectedDiff) > 1e-9 {
				t.Errorf("diff: want %f, got %f", expectedDiff, d.Diff)
			}
		})
	}
}

func TestWriteIterationLog(t *testing.T) {
	dir := t.TempDir()
	lg, err := NewLogger(dir, 7)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer lg.Close()

	content := "agent output for iteration 1"
	if err := lg.WriteIterationLog(1, content); err != nil {
		t.Fatalf("WriteIterationLog: %v", err)
	}

	// Verify a .log file was created with expected content.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".log" {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != content {
				t.Errorf("content mismatch: got %q", string(data))
			}
			found = true
		}
	}
	if !found {
		t.Error("no .log file found")
	}
}
