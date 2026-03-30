package tui

import tea "github.com/charmbracelet/bubbletea"

// Output is the common interface both TUI and plain output implement.
type Output interface {
	// Start the output (for TUI: start bubbletea program; for plain: noop).
	Start() error
	// Send messages to the output.
	Send(msg tea.Msg)
	// Wait for the output to finish.
	Wait()
	// Shutdown cleans up resources.
	Shutdown()
}
