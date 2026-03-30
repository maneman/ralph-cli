package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Header background colors by phase.
var (
	HeaderRunning   = lipgloss.Color("27")  // blue
	HeaderReviewing = lipgloss.Color("220") // yellow
	HeaderSuccess   = lipgloss.Color("34")  // green
	HeaderFailure   = lipgloss.Color("196") // red
)

// Body styles.
var (
	ToolNameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("242")) // dim gray
	ToolOutputBold = lipgloss.NewStyle().Bold(true)
	SeparatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	SummaryKey     = lipgloss.NewStyle().Bold(true)
	SummaryGood    = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	SummaryBad     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

// FormatSeparator renders an iteration separator line.
func FormatSeparator(msg IterationSeparatorMsg, width int) string {
	if width < 10 {
		width = 80
	}
	label := fmt.Sprintf(" Iteration %d │ %s │ %s ",
		msg.Iteration, msg.TaskID, msg.Outcome)

	pad := width - len(label)
	if pad < 2 {
		pad = 2
	}
	left := pad / 2
	right := pad - left

	line := strings.Repeat("─", left) + label + strings.Repeat("─", right)
	return SeparatorStyle.Render(line)
}

// FormatSummary renders the end-of-run summary block.
func FormatSummary(msg RunSummaryMsg) string {
	var b strings.Builder
	b.WriteString(SeparatorStyle.Render(strings.Repeat("═", 60)) + "\n")
	b.WriteString(SummaryKey.Render("Run Summary") + "\n")
	b.WriteString(fmt.Sprintf("  Iterations : %s\n", SummaryKey.Render(fmt.Sprintf("%d", msg.Iterations))))
	b.WriteString(fmt.Sprintf("  Successes  : %s\n", SummaryGood.Render(fmt.Sprintf("%d", msg.Successes))))
	b.WriteString(fmt.Sprintf("  Gaps found : %d\n", msg.GapsFound))
	b.WriteString(fmt.Sprintf("  Blocked    : %s\n", colorByCount(msg.Blocked)))
	b.WriteString(fmt.Sprintf("  Success %%  : %s\n", colorByRate(msg.SuccessRate)))
	b.WriteString(fmt.Sprintf("  Total time : %s\n", formatDuration(msg.TotalTime)))
	b.WriteString(fmt.Sprintf("  Tokens     : %d\n", msg.TotalTokens))
	b.WriteString(fmt.Sprintf("  Cost       : $%.2f\n", msg.TotalCost))
	b.WriteString(SeparatorStyle.Render(strings.Repeat("═", 60)))
	return b.String()
}

func colorByCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if n > 0 {
		return SummaryBad.Render(s)
	}
	return SummaryGood.Render(s)
}

func colorByRate(rate float64) string {
	s := fmt.Sprintf("%.1f%%", rate)
	if rate >= 80 {
		return SummaryGood.Render(s)
	}
	return SummaryBad.Render(s)
}
