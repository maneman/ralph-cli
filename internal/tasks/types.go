package tasks

// Task represents a single row from the task index markdown table.
type Task struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	Dependencies []string `json:"deps,omitempty"`
	BlockReason  string   `json:"blocked_reason,omitempty"`
}

// TaskSpec holds the full markdown content of an individual task spec file.
type TaskSpec struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"full_markdown_content"`
}

// TaskProgress provides counts of tasks by status.
type TaskProgress struct {
	Done       int `json:"done"`
	InProgress int `json:"in_progress"`
	NotStarted int `json:"not_started"`
	Blocked    int `json:"blocked"`
	Total      int `json:"total"`
}
