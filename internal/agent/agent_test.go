package agent

import (
	"testing"
	"time"

	"github.com/mane/ralph-cli/internal/tasks"
)

func TestEventTypeConstants(t *testing.T) {
	// Verify iota ordering produces distinct values.
	types := []EventType{
		EventAssistantDelta,
		EventAssistantIntent,
		EventToolStart,
		EventToolPartialOutput,
		EventToolComplete,
		EventUsage,
		EventSessionIdle,
		EventError,
	}
	seen := make(map[EventType]bool, len(types))
	for _, et := range types {
		if seen[et] {
			t.Fatalf("duplicate EventType value: %d", et)
		}
		seen[et] = true
	}
	if EventAssistantDelta != 0 {
		t.Fatalf("expected EventAssistantDelta == 0, got %d", EventAssistantDelta)
	}
	if EventError != EventType(len(types)-1) {
		t.Fatalf("expected EventError == %d, got %d", len(types)-1, EventError)
	}
}

func TestEventStruct(t *testing.T) {
	ev := Event{
		Type:       EventToolComplete,
		ToolName:   "ralph_list_tasks",
		ToolCallID: "call-1",
		Success:    true,
		Duration:   2 * time.Second,
	}
	if ev.Type != EventToolComplete {
		t.Fatal("unexpected Type")
	}
	if ev.Duration != 2*time.Second {
		t.Fatal("unexpected Duration")
	}
}

func TestSessionConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SessionConfig
		wantErr bool
	}{
		{
			name:    "missing model",
			cfg:     SessionConfig{Callbacks: &ToolCallbacks{}},
			wantErr: true,
		},
		{
			name:    "missing callbacks",
			cfg:     SessionConfig{Model: "gpt-4"},
			wantErr: true,
		},
		{
			name: "valid",
			cfg: SessionConfig{
				Model:     "gpt-4",
				Callbacks: &ToolCallbacks{},
			},
			wantErr: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestToolCallbacksMock(t *testing.T) {
	cb := &ToolCallbacks{
		ListTasks: func() ([]tasks.Task, error) {
			return []tasks.Task{
				{ID: "t1", Title: "task one", Status: "not-started"},
			}, nil
		},
		PickTask: func() (*PickTaskResult, error) {
			return &PickTaskResult{ID: "t1", Title: "task one", SpecContent: "# Task", Attempt: 1}, nil
		},
		GetTaskSpec: func(taskID string) (*tasks.TaskSpec, error) {
			return &tasks.TaskSpec{ID: taskID, Title: "task one", Content: "# Spec"}, nil
		},
		UpdateStatus: func(taskID, status, reason string) error {
			return nil
		},
		ReportOutcome: func(taskID, outcome, summary string) error {
			return nil
		},
	}

	taskList, err := cb.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(taskList) != 1 || taskList[0].ID != "t1" {
		t.Fatalf("unexpected tasks: %+v", taskList)
	}

	pick, err := cb.PickTask()
	if err != nil {
		t.Fatalf("PickTask: %v", err)
	}
	if pick.ID != "t1" {
		t.Fatalf("unexpected pick: %+v", pick)
	}

	spec, err := cb.GetTaskSpec("t1")
	if err != nil {
		t.Fatalf("GetTaskSpec: %v", err)
	}
	if spec.Content != "# Spec" {
		t.Fatalf("unexpected spec: %+v", spec)
	}

	if err := cb.UpdateStatus("t1", "in-progress", ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	if err := cb.ReportOutcome("t1", "success", "done"); err != nil {
		t.Fatalf("ReportOutcome: %v", err)
	}
}

func TestPickTaskResultJSON(t *testing.T) {
	r := PickTaskResult{
		ID:          "task-1",
		Title:       "First task",
		SpecContent: "# Spec content",
		Attempt:     2,
		GapContext:  "retry context",
	}
	if r.ID != "task-1" || r.Attempt != 2 {
		t.Fatalf("unexpected fields: %+v", r)
	}
}
