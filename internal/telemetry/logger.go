package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	outcomesFileName = "ralph-outcomes.jsonl"
	runsFileName     = "ralph-runs.jsonl"
)

// Logger writes structured telemetry data to JSONL files and per-iteration logs.
type Logger struct {
	logsDir       string
	outcomesFile  *os.File
	retentionDays int
}

// NewLogger creates a Logger that writes to logsDir. It opens (or creates)
// the outcomes JSONL file for appending. retentionDays controls how long
// individual iteration log files are kept by CleanOldLogs.
func NewLogger(logsDir string, retentionDays int) (*Logger, error) {
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create logs dir: %w", err)
	}

	outPath := filepath.Join(logsDir, outcomesFileName)
	f, err := os.OpenFile(outPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open outcomes file: %w", err)
	}

	return &Logger{
		logsDir:       logsDir,
		outcomesFile:  f,
		retentionDays: retentionDays,
	}, nil
}

// LogOutcome appends a single IterationOutcome as a JSON line to the outcomes file.
func (l *Logger) LogOutcome(outcome IterationOutcome) error {
	data, err := json.Marshal(outcome)
	if err != nil {
		return fmt.Errorf("marshal outcome: %w", err)
	}
	data = append(data, '\n')
	if _, err := l.outcomesFile.Write(data); err != nil {
		return fmt.Errorf("write outcome: %w", err)
	}
	return nil
}

// LogRun appends a RunSummary as a JSON line to the runs file.
func (l *Logger) LogRun(summary RunSummary) error {
	runsPath := filepath.Join(l.logsDir, runsFileName)
	f, err := os.OpenFile(runsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open runs file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal run summary: %w", err)
	}
	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write run summary: %w", err)
	}
	return nil
}

// WriteIterationLog writes the full agent output for a single iteration to an
// individual log file named ralph-YYYYMMDD-HHMMSS-iteration-N.log.
func (l *Logger) WriteIterationLog(iteration int, content string) error {
	ts := time.Now().UTC().Format("20060102-150405")
	name := fmt.Sprintf("ralph-%s-iteration-%d.log", ts, iteration)
	path := filepath.Join(l.logsDir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write iteration log: %w", err)
	}
	return nil
}

// CleanOldLogs removes log files (*.log) in logsDir that are older than
// retentionDays based on file modification time.
func (l *Logger) CleanOldLogs() error {
	cutoff := time.Now().Add(-time.Duration(l.retentionDays) * 24 * time.Hour)

	entries, err := os.ReadDir(l.logsDir)
	if err != nil {
		return fmt.Errorf("read logs dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(l.logsDir, entry.Name())); err != nil {
				return fmt.Errorf("remove old log %s: %w", entry.Name(), err)
			}
		}
	}
	return nil
}

// Close closes open file handles held by the Logger.
func (l *Logger) Close() error {
	if l.outcomesFile != nil {
		return l.outcomesFile.Close()
	}
	return nil
}
