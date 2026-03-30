package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HeaderState holds the data displayed in the fixed header.
type HeaderState struct {
	Iteration     int
	TotalIter     int
	TaskID        string
	TaskTitle     string
	TaskDuration  time.Duration
	TotalDuration time.Duration
	Done          int
	Remaining     int
	Retries       int
	Blocked       int
	Model         string
	Phase         string // "running", "reviewing", "success", "failure", "idle"
}

// --- Event types the engine sends to the TUI ---

// AgentOutputMsg carries assistant text output.
type AgentOutputMsg struct {
	Content string
}

// ToolStartMsg signals that a tool invocation has begun.
type ToolStartMsg struct {
	ToolName  string
	Arguments string
}

// ToolOutputMsg carries output produced by a tool.
type ToolOutputMsg struct {
	ToolCallID string
	Output     string
}

// ToolCompleteMsg signals that a tool invocation finished.
type ToolCompleteMsg struct {
	ToolCallID string
	ToolName   string
	Success    bool
	Duration   time.Duration
}

// HeaderUpdateMsg replaces the current header state.
type HeaderUpdateMsg struct {
	Header HeaderState
}

// IterationSeparatorMsg marks the boundary between iterations.
type IterationSeparatorMsg struct {
	Iteration int
	TaskID    string
	Duration  time.Duration
	Outcome   string
}

// RunSummaryMsg carries end-of-run summary data.
type RunSummaryMsg struct {
	Iterations  int
	Successes   int
	GapsFound   int
	Blocked     int
	SuccessRate float64
	TotalTime   time.Duration
	TotalTokens int
	TotalCost   float64
}

// --- Bubbletea model ---

const headerHeight = 2

// Model is the main Bubbletea model for the Ralph TUI.
type Model struct {
	header HeaderState
	lines  []string // scrolling body lines
	width  int
	height int
	scroll int // first visible line index in body
	quit   bool
}

// NewModel returns a Model ready for use with bubbletea.
func NewModel() Model {
	return Model{
		header: HeaderState{Phase: "idle"},
		lines:  nil,
		width:  80,
		height: 24,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quit = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampScroll()

	case HeaderUpdateMsg:
		m.header = msg.Header

	case AgentOutputMsg:
		m.appendLines(msg.Content)

	case ToolStartMsg:
		line := fmt.Sprintf("⚙ %s %s", msg.ToolName, msg.Arguments)
		m.appendLines(line)

	case ToolOutputMsg:
		m.appendLines(msg.Output)

	case ToolCompleteMsg:
		status := "✓"
		if !msg.Success {
			status = "✗"
		}
		line := fmt.Sprintf("%s %s (%s)", status, msg.ToolName, msg.Duration.Truncate(time.Millisecond))
		m.appendLines(line)

	case IterationSeparatorMsg:
		m.appendLines(FormatSeparator(msg, m.width))

	case RunSummaryMsg:
		m.appendLines(FormatSummary(msg))
	}

	return m, nil
}

func (m *Model) appendLines(text string) {
	for _, line := range strings.Split(text, "\n") {
		m.lines = append(m.lines, line)
	}
	m.scrollToBottom()
}

func (m *Model) scrollToBottom() {
	bodyH := m.bodyHeight()
	if len(m.lines) > bodyH {
		m.scroll = len(m.lines) - bodyH
	} else {
		m.scroll = 0
	}
}

func (m *Model) clampScroll() {
	bodyH := m.bodyHeight()
	max := len(m.lines) - bodyH
	if max < 0 {
		max = 0
	}
	if m.scroll > max {
		m.scroll = max
	}
}

func (m Model) bodyHeight() int {
	h := m.height - headerHeight
	if h < 1 {
		h = 1
	}
	return h
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quit {
		return ""
	}

	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString(m.renderBody())
	return b.String()
}

// renderHeader produces the fixed 2-line header.
func (m Model) renderHeader() string {
	h := m.header
	style := headerStyleForPhase(h.Phase, m.width)

	line1 := fmt.Sprintf("▶ Ralph #%d/%d │ %s: %s │ ⏱ %s │ Total: %s",
		h.Iteration, h.TotalIter,
		h.TaskID, h.TaskTitle,
		formatDuration(h.TaskDuration),
		formatDuration(h.TotalDuration),
	)

	line2 := fmt.Sprintf("✓ %d done │ %d remaining │ ↻ %d retry │ ■ %d blocked │ %s",
		h.Done, h.Remaining, h.Retries, h.Blocked, h.Model,
	)

	return style.Render(line1) + "\n" + style.Render(line2) + "\n"
}

func headerStyleForPhase(phase string, width int) lipgloss.Style {
	base := lipgloss.NewStyle().Width(width).Bold(true)

	switch phase {
	case "running":
		return base.Background(HeaderRunning).Foreground(lipgloss.Color("15"))
	case "reviewing":
		return base.Background(HeaderReviewing).Foreground(lipgloss.Color("0"))
	case "success":
		return base.Background(HeaderSuccess).Foreground(lipgloss.Color("15"))
	case "failure":
		return base.Background(HeaderFailure).Foreground(lipgloss.Color("15"))
	default:
		return base.Background(lipgloss.Color("240")).Foreground(lipgloss.Color("15"))
	}
}

// renderBody produces the scrolling output area.
func (m Model) renderBody() string {
	bodyH := m.bodyHeight()
	end := m.scroll + bodyH
	if end > len(m.lines) {
		end = len(m.lines)
	}

	var b strings.Builder
	start := m.scroll
	if start < 0 {
		start = 0
	}
	for i := start; i < end; i++ {
		b.WriteString(m.lines[i])
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// --- TUI Output adapter ---

// TUIOutput wraps a bubbletea Program and implements Output.
type TUIOutput struct {
	program *tea.Program
	model   Model
	done    chan struct{}
}

// NewTUIOutput creates a new TUIOutput.
func NewTUIOutput() *TUIOutput {
	return &TUIOutput{
		done: make(chan struct{}),
	}
}

// Start launches the bubbletea program.
func (t *TUIOutput) Start() error {
	t.model = NewModel()
	t.program = tea.NewProgram(t.model, tea.WithAltScreen())

	go func() {
		defer close(t.done)
		if _, err := t.program.Run(); err != nil {
			fmt.Printf("TUI error: %v\n", err)
		}
	}()
	return nil
}

// Send delivers a message to the running program.
func (t *TUIOutput) Send(msg tea.Msg) {
	if t.program != nil {
		t.program.Send(msg)
	}
}

// Wait blocks until the TUI exits.
func (t *TUIOutput) Wait() {
	<-t.done
}

// Shutdown asks the program to quit.
func (t *TUIOutput) Shutdown() {
	if t.program != nil {
		t.program.Quit()
	}
}

// formatDuration renders a duration as e.g. "2m 14s".
func formatDuration(d time.Duration) string {
	d = d.Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", m, s)
}
