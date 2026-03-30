package agent

import "time"

// EventType discriminates agent events forwarded to the TUI/engine.
type EventType int

const (
	EventAssistantDelta EventType = iota
	EventAssistantIntent
	EventToolStart
	EventToolPartialOutput
	EventToolComplete
	EventUsage
	EventSessionIdle
	EventError
)

// Event is emitted by the agent session for TUI/engine consumption.
type Event struct {
	Type       EventType
	Content    string        // delta text, partial output, error message
	ToolCallID string        // for tool events
	ToolName   string        // for tool start/complete
	Arguments  string        // for tool start (JSON string)
	Success    bool          // for tool complete
	Duration   time.Duration // for tool complete
	Tokens     int           // for usage
	Cost       float64       // for usage
	Model      string        // for usage
}
