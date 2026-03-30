package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	copilot "github.com/github/copilot-sdk/go"
)

// SessionConfig configures a single agent session.
type SessionConfig struct {
	Model       string         // model identifier (required)
	Prompt      string         // full constructed prompt (required)
	Callbacks   *ToolCallbacks // tool callbacks (required)
	OnEvent     func(Event)    // streaming event callback
	AutoApprove bool           // approve all permission requests
}

// Validate checks that the required fields are populated.
func (c *SessionConfig) Validate() error {
	if c.Model == "" {
		return errors.New("agent: Model is required")
	}
	if c.Callbacks == nil {
		return errors.New("agent: Callbacks is required")
	}
	return nil
}

// RunResult is the outcome of a completed agent session.
type RunResult struct {
	TaskID   string
	Outcome  string // "success", "stuck", "blocked"
	Summary  string
	Tokens   int
	Cost     float64
	Duration time.Duration
}

// Run creates an SDK session, sends the prompt, and blocks until the session
// completes. Events are streamed via OnEvent. Returns when the agent calls
// ralph_report_outcome or the session becomes idle.
func Run(ctx context.Context, cfg SessionConfig) (*RunResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	emit := cfg.OnEvent
	if emit == nil {
		emit = func(Event) {}
	}

	// Outcome capture — set when ralph_report_outcome fires.
	var (
		mu       sync.Mutex
		outcome  *RunResult
		doneCh   = make(chan struct{})
		doneOnce sync.Once
	)

	// Wrap ReportOutcome to capture the result and signal completion.
	origReport := cfg.Callbacks.ReportOutcome
	cfg.Callbacks.ReportOutcome = func(taskID, out, summary string) error {
		if origReport != nil {
			if err := origReport(taskID, out, summary); err != nil {
				return err
			}
		}
		mu.Lock()
		outcome = &RunResult{
			TaskID:  taskID,
			Outcome: out,
			Summary: summary,
		}
		mu.Unlock()
		doneOnce.Do(func() { close(doneCh) })
		return nil
	}

	// Build tools.
	tools := buildTools(cfg.Callbacks)

	// Permission handler.
	var permHandler copilot.PermissionHandlerFunc
	if cfg.AutoApprove {
		permHandler = copilot.PermissionHandler.ApproveAll
	} else {
		permHandler = copilot.PermissionHandler.ApproveAll
	}

	start := time.Now()

	// Create SDK client.
	client := copilot.NewClient(nil)
	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("agent: failed to start copilot client: %w", err)
	}
	defer client.Stop()

	// Event handler — translate SDK events to our Event type.
	onEvent := func(ev copilot.SessionEvent) {
		switch ev.Type {
		case copilot.SessionEventTypeAssistantMessageDelta:
			if ev.Data.DeltaContent != nil {
				emit(Event{Type: EventAssistantDelta, Content: *ev.Data.DeltaContent})
			}
		case copilot.SessionEventTypeAssistantIntent:
			if ev.Data.Intent != nil {
				emit(Event{Type: EventAssistantIntent, Content: *ev.Data.Intent})
			}
		case copilot.SessionEventTypeToolExecutionStart:
			var args string
			if ev.Data.Arguments != nil {
				if b, err := json.Marshal(ev.Data.Arguments); err == nil {
					args = string(b)
				}
			}
			emit(Event{
				Type:       EventToolStart,
				ToolName:   ptrStr(ev.Data.ToolName),
				Arguments:  args,
				ToolCallID: ptrStr(ev.Data.ToolCallID),
			})
		case copilot.SessionEventTypeToolExecutionPartialResult:
			emit(Event{
				Type:       EventToolPartialOutput,
				Content:    ptrStr(ev.Data.PartialOutput),
				ToolCallID: ptrStr(ev.Data.ToolCallID),
			})
		case copilot.SessionEventTypeToolExecutionComplete:
			var dur time.Duration
			if ev.Data.Duration != nil {
				dur = time.Duration(*ev.Data.Duration) * time.Millisecond
			}
			emit(Event{
				Type:       EventToolComplete,
				ToolName:   ptrStr(ev.Data.ToolName),
				ToolCallID: ptrStr(ev.Data.ToolCallID),
				Success:    ev.Data.Success != nil && *ev.Data.Success,
				Duration:   dur,
			})
		case copilot.SessionEventTypeAssistantUsage:
			tokens := 0
			if ev.Data.InputTokens != nil {
				tokens += int(*ev.Data.InputTokens)
			}
			if ev.Data.OutputTokens != nil {
				tokens += int(*ev.Data.OutputTokens)
			}
			var cost float64
			if ev.Data.Cost != nil {
				cost = *ev.Data.Cost
			}
			emit(Event{
				Type:   EventUsage,
				Tokens: tokens,
				Cost:   cost,
				Model:  ptrStr(ev.Data.Model),
			})
		case copilot.SessionEventTypeSessionIdle:
			emit(Event{Type: EventSessionIdle})
			doneOnce.Do(func() { close(doneCh) })
		case copilot.SessionEventTypeSessionError:
			msg := "unknown error"
			if ev.Data.Message != nil {
				msg = *ev.Data.Message
			}
			emit(Event{Type: EventError, Content: msg})
		}
	}

	// Create session.
	session, err := client.CreateSession(ctx, &copilot.SessionConfig{
		Model:               cfg.Model,
		Streaming:           true,
		Tools:               tools,
		OnPermissionRequest: permHandler,
		OnEvent:             onEvent,
	})
	if err != nil {
		return nil, fmt.Errorf("agent: failed to create session: %w", err)
	}
	defer session.Disconnect()

	// Send the prompt.
	if _, err := session.Send(ctx, copilot.MessageOptions{Prompt: cfg.Prompt}); err != nil {
		return nil, fmt.Errorf("agent: failed to send prompt: %w", err)
	}

	// Wait for completion (outcome report or session idle) or context cancellation.
	select {
	case <-doneCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	elapsed := time.Since(start)

	mu.Lock()
	result := outcome
	mu.Unlock()

	if result == nil {
		result = &RunResult{Outcome: "stuck", Summary: "session ended without outcome report"}
	}
	result.Duration = elapsed
	return result, nil
}

// ptrStr safely dereferences a *string, returning "" for nil.
func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
