package telemetry

import "time"

// IterationOutcome is a per-iteration outcome record.
type IterationOutcome struct {
	Iteration int           `json:"iteration"`
	TaskID    string        `json:"task_id"`
	Outcome   string        `json:"outcome"` // SUCCESS, GAPS_FOUND, NO_OP, AGENT_CRASH, STUCK, PRD_COMPLETE
	Duration  time.Duration `json:"duration_ms"`
	Attempt   int           `json:"attempt"`
	Commits   int           `json:"commits"`
	Tokens    int           `json:"tokens,omitempty"`
	Cost      float64       `json:"cost,omitempty"`
	GapCount  int           `json:"gap_count,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// RunSummary is a per-run aggregate summary.
type RunSummary struct {
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	TotalTime   time.Duration `json:"total_time_ms"`
	Iterations  int           `json:"iterations"`
	Completed   int           `json:"completed"`
	Successes   int           `json:"successes"`
	GapsFound   int           `json:"gaps_found"`
	NoOps       int           `json:"no_ops"`
	Crashes     int           `json:"crashes"`
	Blocked     int           `json:"blocked"`
	PRDComplete bool          `json:"prd_complete"`
	SuccessRate float64       `json:"success_rate"`
	GapRate     float64       `json:"gap_rate"`
	AvgRetries  float64       `json:"avg_retries"`
	TotalTokens int           `json:"total_tokens"`
	TotalCost   float64       `json:"total_cost"`
}

// BaselineComparison holds a comparison against a rolling average baseline.
type BaselineComparison struct {
	SuccessRate Delta `json:"success_rate"`
	GapRate     Delta `json:"gap_rate"`
	AvgDuration Delta `json:"avg_duration"`
	AvgRetries  Delta `json:"avg_retries"`
}

// Delta represents the difference between a current value and a baseline.
type Delta struct {
	Current  float64 `json:"current"`
	Baseline float64 `json:"baseline"`
	Diff     float64 `json:"diff"`   // current - baseline
	Symbol   string  `json:"symbol"` // "▲" or "▼" or "─"
}
