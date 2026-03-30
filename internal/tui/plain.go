package tui

import (
	"fmt"
	"io"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// PlainOutput implements Output by printing directly to stdout with no
// ANSI cursor manipulation. Suitable for --no-tui mode or piped output.
type PlainOutput struct {
	w io.Writer
}

// NewPlainOutput returns a PlainOutput that writes to stdout.
func NewPlainOutput() *PlainOutput {
	return &PlainOutput{w: os.Stdout}
}

// newPlainOutputWriter returns a PlainOutput writing to w (for testing).
func newPlainOutputWriter(w io.Writer) *PlainOutput {
	return &PlainOutput{w: w}
}

// Start is a no-op for plain output.
func (p *PlainOutput) Start() error { return nil }

// Send dispatches a message and prints the appropriate output.
func (p *PlainOutput) Send(msg tea.Msg) {
	switch msg := msg.(type) {
	case HeaderUpdateMsg:
		p.printHeader(msg.Header)

	case AgentOutputMsg:
		fmt.Fprintln(p.w, msg.Content)

	case ToolStartMsg:
		fmt.Fprintf(p.w, "[tool] %s %s\n", msg.ToolName, msg.Arguments)

	case ToolOutputMsg:
		fmt.Fprintln(p.w, msg.Output)

	case ToolCompleteMsg:
		status := "ok"
		if !msg.Success {
			status = "FAIL"
		}
		fmt.Fprintf(p.w, "[tool] %s %s (%s)\n", msg.ToolName, status, msg.Duration.Truncate(time.Millisecond))

	case IterationSeparatorMsg:
		fmt.Fprintf(p.w, "--- Iteration %d │ %s │ %s ---\n",
			msg.Iteration, msg.TaskID, msg.Outcome)

	case RunSummaryMsg:
		fmt.Fprintf(p.w, "=== Run Summary: %d iterations, %d successes, %.1f%% rate, %s total ===\n",
			msg.Iterations, msg.Successes, msg.SuccessRate, formatDuration(msg.TotalTime))
	}
}

func (p *PlainOutput) printHeader(h HeaderState) {
	fmt.Fprintf(p.w, "▶ Ralph #%d/%d │ %s: %s │ ⏱ %s │ Total: %s │ ✓%d remaining:%d retry:%d blocked:%d │ %s\n",
		h.Iteration, h.TotalIter,
		h.TaskID, h.TaskTitle,
		formatDuration(h.TaskDuration),
		formatDuration(h.TotalDuration),
		h.Done, h.Remaining, h.Retries, h.Blocked,
		h.Model,
	)
}

// Wait is a no-op for plain output.
func (p *PlainOutput) Wait() {}

// Shutdown is a no-op for plain output.
func (p *PlainOutput) Shutdown() {}
