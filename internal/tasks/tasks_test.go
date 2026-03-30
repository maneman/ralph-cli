package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleIndex = `# Task Index

| ID | Title | Status | Dependencies |
|----|-------|--------|-------------|
| T001 | Set up auth module | done | |
| T002 | Add JWT validation | in-progress | T001 |
| T003 | Create user API | not-started | T001 |
| T004 | Add rate limiting | not-started | T002, T003 |
| T005 | Write integration tests | blocked | T004 |
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadIndex(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "index.md", sampleIndex)

	idx, err := LoadIndex(path, `T\d+`)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	tasks := idx.ListTasks()
	if len(tasks) != 5 {
		t.Fatalf("expected 5 tasks, got %d", len(tasks))
	}

	// Verify first task.
	if tasks[0].ID != "T001" {
		t.Errorf("expected ID T001, got %s", tasks[0].ID)
	}
	if tasks[0].Title != "Set up auth module" {
		t.Errorf("expected title 'Set up auth module', got %q", tasks[0].Title)
	}
	if tasks[0].Status != "done" {
		t.Errorf("expected status done, got %s", tasks[0].Status)
	}
	if len(tasks[0].Dependencies) != 0 {
		t.Errorf("expected no deps for T001, got %v", tasks[0].Dependencies)
	}

	// Verify task with dependencies.
	if tasks[3].ID != "T004" {
		t.Errorf("expected ID T004, got %s", tasks[3].ID)
	}
	if len(tasks[3].Dependencies) != 2 {
		t.Fatalf("expected 2 deps for T004, got %v", tasks[3].Dependencies)
	}
	if tasks[3].Dependencies[0] != "T002" || tasks[3].Dependencies[1] != "T003" {
		t.Errorf("unexpected deps for T004: %v", tasks[3].Dependencies)
	}
}

func TestLoadIndexEmptyTable(t *testing.T) {
	dir := t.TempDir()
	content := `# Task Index

| ID | Title | Status | Dependencies |
|----|-------|--------|-------------|
`
	path := writeFile(t, dir, "index.md", content)

	idx, err := LoadIndex(path, `T\d+`)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if len(idx.ListTasks()) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(idx.ListTasks()))
	}
}

func TestLoadIndexExtraWhitespace(t *testing.T) {
	dir := t.TempDir()
	content := `# Task Index

|  ID  |  Title  |  Status  |  Dependencies  |
|------|---------|----------|----------------|
|  T001  |  Auth module  |  done  |    |
|  T002  |  JWT  |  in-progress  |  T001  |
`
	path := writeFile(t, dir, "index.md", content)

	idx, err := LoadIndex(path, `T\d+`)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	tasks := idx.ListTasks()
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Title != "Auth module" {
		t.Errorf("expected trimmed title, got %q", tasks[0].Title)
	}
}

func TestLoadIndexMissingColumns(t *testing.T) {
	dir := t.TempDir()
	// Missing Status column.
	content := `| ID | Title |
|----|-------|
| T001 | Foo |
`
	path := writeFile(t, dir, "index.md", content)

	_, err := LoadIndex(path, `T\d+`)
	if err == nil {
		t.Fatal("expected error for missing columns, got nil")
	}
}

func TestNextEligible(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "index.md", sampleIndex)

	idx, err := LoadIndex(path, `T\d+`)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	// T003 is not-started and depends on T001 (done) → eligible.
	next := idx.NextEligible()
	if next == nil {
		t.Fatal("expected a next eligible task, got nil")
	}
	if next.ID != "T003" {
		t.Errorf("expected T003, got %s", next.ID)
	}
}

func TestNextEligibleAllDone(t *testing.T) {
	dir := t.TempDir()
	content := `| ID | Title | Status | Dependencies |
|----|-------|--------|-------------|
| T001 | A | done | |
| T002 | B | done | T001 |
`
	path := writeFile(t, dir, "index.md", content)

	idx, err := LoadIndex(path, `T\d+`)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	next := idx.NextEligible()
	if next != nil {
		t.Errorf("expected nil when all done, got %s", next.ID)
	}
}

func TestNextEligibleAllBlocked(t *testing.T) {
	dir := t.TempDir()
	content := `| ID | Title | Status | Dependencies |
|----|-------|--------|-------------|
| T001 | A | in-progress | |
| T002 | B | not-started | T001 |
`
	path := writeFile(t, dir, "index.md", content)

	idx, err := LoadIndex(path, `T\d+`)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	next := idx.NextEligible()
	if next != nil {
		t.Errorf("expected nil when deps not done, got %s", next.ID)
	}
}

func TestUpdateStatus(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "index.md", sampleIndex)

	idx, err := LoadIndex(path, `T\d+`)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	// Update T003 from not-started to in-progress.
	if err := idx.UpdateStatus("T003", "in-progress", ""); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify in memory.
	task := idx.FindTask("T003")
	if task == nil {
		t.Fatal("T003 not found after update")
	}
	if task.Status != "in-progress" {
		t.Errorf("expected in-progress, got %s", task.Status)
	}

	// Verify on disk by re-loading.
	idx2, err := LoadIndex(path, `T\d+`)
	if err != nil {
		t.Fatalf("reloading index failed: %v", err)
	}
	task2 := idx2.FindTask("T003")
	if task2 == nil {
		t.Fatal("T003 not found after reload")
	}
	if task2.Status != "in-progress" {
		t.Errorf("expected in-progress on disk, got %s", task2.Status)
	}
}

func TestUpdateStatusInvalid(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "index.md", sampleIndex)

	idx, err := LoadIndex(path, `T\d+`)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	if err := idx.UpdateStatus("T001", "invalid-status", ""); err == nil {
		t.Fatal("expected error for invalid status")
	}

	if err := idx.UpdateStatus("T999", "done", ""); err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestUpdateStatusPreservesFileStructure(t *testing.T) {
	dir := t.TempDir()
	content := `# My Project Tasks

Some description here.

| ID | Title | Status | Dependencies |
|----|-------|--------|-------------|
| T001 | First | done | |
| T002 | Second | not-started | T001 |

## Notes

Some notes below the table.
`
	path := writeFile(t, dir, "index.md", content)

	idx, err := LoadIndex(path, `T\d+`)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	if err := idx.UpdateStatus("T002", "in-progress", ""); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := string(data)

	if !strings.Contains(result, "# My Project Tasks") {
		t.Error("title was lost")
	}
	if !strings.Contains(result, "Some description here.") {
		t.Error("description was lost")
	}
	if !strings.Contains(result, "## Notes") {
		t.Error("notes section was lost")
	}
	if !strings.Contains(result, "Some notes below the table.") {
		t.Error("notes content was lost")
	}
}

func TestFindTask(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "index.md", sampleIndex)

	idx, err := LoadIndex(path, `T\d+`)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	task := idx.FindTask("T002")
	if task == nil {
		t.Fatal("expected to find T002")
	}
	if task.Title != "Add JWT validation" {
		t.Errorf("wrong title: %s", task.Title)
	}

	if idx.FindTask("T999") != nil {
		t.Error("expected nil for non-existent task")
	}
}

func TestProgress(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "index.md", sampleIndex)

	idx, err := LoadIndex(path, `T\d+`)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	p := idx.Progress()
	if p.Total != 5 {
		t.Errorf("expected total 5, got %d", p.Total)
	}
	if p.Done != 1 {
		t.Errorf("expected done 1, got %d", p.Done)
	}
	if p.InProgress != 1 {
		t.Errorf("expected in-progress 1, got %d", p.InProgress)
	}
	if p.NotStarted != 2 {
		t.Errorf("expected not-started 2, got %d", p.NotStarted)
	}
	if p.Blocked != 1 {
		t.Errorf("expected blocked 1, got %d", p.Blocked)
	}
}

func TestFindTaskSpec(t *testing.T) {
	dir := t.TempDir()

	specContent := "# Auth Module\n\nImplement JWT-based auth.\n"
	writeFile(t, dir, "T001-set-up-auth-module.md", specContent)
	writeFile(t, dir, "T002-add-jwt-validation.md", "# JWT\n")

	spec, err := FindTaskSpec(dir, "T001")
	if err != nil {
		t.Fatalf("FindTaskSpec failed: %v", err)
	}
	if spec.ID != "T001" {
		t.Errorf("expected ID T001, got %s", spec.ID)
	}
	if spec.Title != "Set up auth module" {
		t.Errorf("expected title 'Set up auth module', got %q", spec.Title)
	}
	if spec.Content != specContent {
		t.Errorf("content mismatch: got %q", spec.Content)
	}
}

func TestFindTaskSpecCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "t001-auth.md", "# Auth\n")

	spec, err := FindTaskSpec(dir, "T001")
	if err != nil {
		t.Fatalf("FindTaskSpec case-insensitive failed: %v", err)
	}
	if spec.ID != "T001" {
		t.Errorf("expected ID T001, got %s", spec.ID)
	}
}

func TestFindTaskSpecNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := FindTaskSpec(dir, "T999")
	if err == nil {
		t.Fatal("expected error for missing spec")
	}
}
