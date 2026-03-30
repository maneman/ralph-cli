package agent

import (
	"encoding/json"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/mane/ralph-cli/internal/tasks"
)

// ToolCallbacks are implemented by the engine to handle tool invocations.
type ToolCallbacks struct {
	ListTasks     func() ([]tasks.Task, error)
	PickTask      func() (*PickTaskResult, error)
	GetTaskSpec   func(taskID string) (*tasks.TaskSpec, error)
	UpdateStatus  func(taskID string, status string, reason string) error
	ReportOutcome func(taskID string, outcome string, summary string) error
}

// PickTaskResult is returned by the ralph_pick_task tool.
type PickTaskResult struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	SpecContent string `json:"spec_content"`
	Attempt     int    `json:"attempt"`
	GapContext  string `json:"gap_context,omitempty"`
}

// Param structs for DefineTool schema generation.

type listTasksParams struct{}

type pickTaskParams struct{}

type getTaskSpecParams struct {
	TaskID string `json:"task_id" jsonschema:"description=ID of the task to retrieve"`
}

type updateStatusParams struct {
	TaskID string `json:"task_id" jsonschema:"description=ID of the task to update"`
	Status string `json:"status"  jsonschema:"description=New status: in-progress, done, or blocked"`
	Reason string `json:"reason"  jsonschema:"description=Optional reason for the status change"`
}

type reportOutcomeParams struct {
	TaskID  string `json:"task_id" jsonschema:"description=ID of the completed task"`
	Outcome string `json:"outcome" jsonschema:"description=Outcome: success, stuck, or blocked"`
	Summary string `json:"summary" jsonschema:"description=Brief summary of work done"`
}

// buildTools constructs the five custom tools from the provided callbacks.
func buildTools(cb *ToolCallbacks) []copilot.Tool {
	return []copilot.Tool{
		copilot.DefineTool("ralph_list_tasks",
			"List all tasks from the task index with their current status.",
			func(_ listTasksParams, _ copilot.ToolInvocation) (any, error) {
				taskList, err := cb.ListTasks()
				if err != nil {
					return nil, err
				}
				b, err := json.Marshal(taskList)
				if err != nil {
					return nil, err
				}
				return string(b), nil
			},
		),

		copilot.DefineTool("ralph_pick_task",
			"Pick the next eligible task to work on. Returns the task spec and gap context if applicable.",
			func(_ pickTaskParams, _ copilot.ToolInvocation) (any, error) {
				result, err := cb.PickTask()
				if err != nil {
					return nil, err
				}
				b, err := json.Marshal(result)
				if err != nil {
					return nil, err
				}
				return string(b), nil
			},
		),

		copilot.DefineTool("ralph_get_task_spec",
			"Get the full spec (markdown content) for a specific task.",
			func(p getTaskSpecParams, _ copilot.ToolInvocation) (any, error) {
				spec, err := cb.GetTaskSpec(p.TaskID)
				if err != nil {
					return nil, err
				}
				b, err := json.Marshal(spec)
				if err != nil {
					return nil, err
				}
				return string(b), nil
			},
		),

		copilot.DefineTool("ralph_update_status",
			"Update the status of a task (in-progress, done, or blocked) with an optional reason.",
			func(p updateStatusParams, _ copilot.ToolInvocation) (any, error) {
				if err := cb.UpdateStatus(p.TaskID, p.Status, p.Reason); err != nil {
					return nil, err
				}
				return "ok", nil
			},
		),

		copilot.DefineTool("ralph_report_outcome",
			"Report the final outcome of the current task iteration. Calling this ends the session.",
			func(p reportOutcomeParams, _ copilot.ToolInvocation) (any, error) {
				if err := cb.ReportOutcome(p.TaskID, p.Outcome, p.Summary); err != nil {
					return nil, err
				}
				return "ok", nil
			},
		),
	}
}
