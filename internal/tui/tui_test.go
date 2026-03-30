package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHeaderRendering(t *testing.T) {
	m := NewModel()
	m.width = 100
	m.height = 24
	m.header = HeaderState{
		Iteration:     3,
		TotalIter:     50,
		TaskID:        "T005",
		TaskTitle:     "Add auth middleware",
		TaskDuration:  2*time.Minute + 14*time.Second,
		TotalDuration: 18*time.Minute + 32*time.Second,
		Done:          4,
		Remaining:     12,
		Retries:       1,
		Blocked:       0,
		Model:         "claude-sonnet-4",
		Phase:         "running",
	}

	view := m.View()

	checks := []string{
		"#3/50",
		"T005",
		"Add auth middleware",
		"2m 14s",
		"18m 32s",
		"4 done",
		"12 remaining",
		"1 retry",
		"0 blocked",
		"claude-sonnet-4",
	}
	for _, want := range checks {
		if !strings.Contains(view, want) {
			t.Errorf("header missing %q in view:\n%s", want, view)
		}
	}
}

func TestModelWindowResize(t *testing.T) {
	m := NewModel()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := updated.(Model)

	if model.width != 120 {
		t.Errorf("width = %d, want 120", model.width)
	}
	if model.height != 40 {
		t.Errorf("height = %d, want 40", model.height)
	}
}

func TestModelAgentOutput(t *testing.T) {
	m := NewModel()
	m.width = 80
	m.height = 24

	updated, _ := m.Update(AgentOutputMsg{Content: "Hello from agent"})
	model := updated.(Model)

	if len(model.lines) == 0 {
		t.Fatal("expected lines after AgentOutputMsg")
	}
	if model.lines[0] != "Hello from agent" {
		t.Errorf("got %q, want %q", model.lines[0], "Hello from agent")
	}
}

func TestModelToolMessages(t *testing.T) {
	m := NewModel()
	m.width = 80
	m.height = 24

	updated, _ := m.Update(ToolStartMsg{ToolName: "bash", Arguments: "ls -la"})
	model := updated.(Model)

	found := false
	for _, l := range model.lines {
		if strings.Contains(l, "bash") && strings.Contains(l, "ls -la") {
			found = true
			break
		}
	}
	if !found {
		t.Error("ToolStartMsg not rendered in lines")
	}

	updated, _ = model.Update(ToolCompleteMsg{
		ToolName: "bash",
		Success:  true,
		Duration: 150 * time.Millisecond,
	})
	model = updated.(Model)

	found = false
	for _, l := range model.lines {
		if strings.Contains(l, "✓") && strings.Contains(l, "bash") {
			found = true
			break
		}
	}
	if !found {
		t.Error("ToolCompleteMsg success not rendered")
	}
}

func TestIterationSeparatorFormatting(t *testing.T) {
	sep := FormatSeparator(IterationSeparatorMsg{
		Iteration: 5,
		TaskID:    "T012",
		Outcome:   "success",
	}, 80)

	if !strings.Contains(sep, "Iteration 5") {
		t.Errorf("separator missing iteration number: %s", sep)
	}
	if !strings.Contains(sep, "T012") {
		t.Errorf("separator missing task ID: %s", sep)
	}
	if !strings.Contains(sep, "success") {
		t.Errorf("separator missing outcome: %s", sep)
	}
}

func TestFormatSummary(t *testing.T) {
	summary := FormatSummary(RunSummaryMsg{
		Iterations:  10,
		Successes:   8,
		GapsFound:   2,
		Blocked:     0,
		SuccessRate: 80.0,
		TotalTime:   45 * time.Minute,
		TotalTokens: 150000,
		TotalCost:   12.50,
	})

	checks := []string{"Run Summary", "10", "8", "80.0%", "45m 0s", "150000", "$12.50"}
	for _, want := range checks {
		if !strings.Contains(summary, want) {
			t.Errorf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestPlainOutputHeader(t *testing.T) {
	var buf bytes.Buffer
	p := newPlainOutputWriter(&buf)

	p.Send(HeaderUpdateMsg{Header: HeaderState{
		Iteration:     1,
		TotalIter:     10,
		TaskID:        "T001",
		TaskTitle:     "Setup",
		TaskDuration:  30 * time.Second,
		TotalDuration: 30 * time.Second,
		Done:          0,
		Remaining:     10,
		Model:         "claude-sonnet-4",
		Phase:         "running",
	}})

	out := buf.String()
	for _, want := range []string{"#1/10", "T001", "Setup", "30s", "claude-sonnet-4"} {
		if !strings.Contains(out, want) {
			t.Errorf("plain header missing %q: %s", want, out)
		}
	}
}

func TestPlainOutputAgentAndTool(t *testing.T) {
	var buf bytes.Buffer
	p := newPlainOutputWriter(&buf)

	p.Send(AgentOutputMsg{Content: "thinking..."})
	p.Send(ToolStartMsg{ToolName: "edit", Arguments: "file.go"})
	p.Send(ToolCompleteMsg{ToolName: "edit", Success: true, Duration: 200 * time.Millisecond})

	out := buf.String()
	if !strings.Contains(out, "thinking...") {
		t.Error("plain missing agent output")
	}
	if !strings.Contains(out, "[tool] edit file.go") {
		t.Errorf("plain missing tool start: %s", out)
	}
	if !strings.Contains(out, "[tool] edit ok") {
		t.Errorf("plain missing tool complete: %s", out)
	}
}

func TestPlainOutputIterationSeparator(t *testing.T) {
	var buf bytes.Buffer
	p := newPlainOutputWriter(&buf)

	p.Send(IterationSeparatorMsg{
		Iteration: 3,
		TaskID:    "T007",
		Outcome:   "failure",
	})

	out := buf.String()
	if !strings.Contains(out, "Iteration 3") || !strings.Contains(out, "T007") || !strings.Contains(out, "failure") {
		t.Errorf("plain separator wrong: %s", out)
	}
}

func TestPlainOutputRunSummary(t *testing.T) {
	var buf bytes.Buffer
	p := newPlainOutputWriter(&buf)

	p.Send(RunSummaryMsg{
		Iterations:  5,
		Successes:   4,
		SuccessRate: 80.0,
		TotalTime:   10 * time.Minute,
	})

	out := buf.String()
	if !strings.Contains(out, "5 iterations") || !strings.Contains(out, "4 successes") {
		t.Errorf("plain summary wrong: %s", out)
	}
}

func TestOutputInterface(t *testing.T) {
	// Ensure both types satisfy Output.
	var _ Output = &TUIOutput{}
	var _ Output = &PlainOutput{}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{2*time.Minute + 14*time.Second, "2m 14s"},
		{1 * time.Hour, "60m 0s"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestModelScrollToBottom(t *testing.T) {
	m := NewModel()
	m.width = 80
	m.height = 5 // bodyHeight = 3

	// Add more lines than body can show
	for i := 0; i < 10; i++ {
		updated, _ := m.Update(AgentOutputMsg{Content: "line"})
		m = updated.(Model)
	}

	if m.scroll != 7 { // 10 lines - 3 bodyHeight = 7
		t.Errorf("scroll = %d, want 7", m.scroll)
	}
}

func TestHeaderPhaseStyles(t *testing.T) {
	phases := []string{"running", "reviewing", "success", "failure", "idle"}
	for _, phase := range phases {
		// Just ensure no panic.
		_ = headerStyleForPhase(phase, 80)
	}
}

func TestModelCtrlCQuits(t *testing.T) {
	m := NewModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("expected quit command on ctrl+c")
	}
}
