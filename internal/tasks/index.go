package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var validStatuses = map[string]bool{
	"not-started": true,
	"in-progress": true,
	"done":        true,
	"blocked":     true,
}

// Index holds parsed tasks from a markdown task index file.
type Index struct {
	tasks     []Task
	filePath  string
	idPattern *regexp.Regexp
}

// LoadIndex parses a markdown file containing a pipe-delimited task table.
// idPattern is a regex string used to validate task IDs (e.g. `T\d+`).
func LoadIndex(indexPath string, idPattern string) (*Index, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("reading index file: %w", err)
	}

	pat, err := regexp.Compile(idPattern)
	if err != nil {
		return nil, fmt.Errorf("compiling id pattern: %w", err)
	}

	idx := &Index{
		filePath:  indexPath,
		idPattern: pat,
	}

	tasks, err := parseTable(string(data), pat)
	if err != nil {
		return nil, fmt.Errorf("parsing index table: %w", err)
	}
	idx.tasks = tasks
	return idx, nil
}

// ListTasks returns all parsed tasks.
func (idx *Index) ListTasks() []Task {
	out := make([]Task, len(idx.tasks))
	copy(out, idx.tasks)
	return out
}

// FindTask returns a pointer to the task with the given ID, or nil.
func (idx *Index) FindTask(taskID string) *Task {
	for i := range idx.tasks {
		if idx.tasks[i].ID == taskID {
			t := idx.tasks[i]
			return &t
		}
	}
	return nil
}

// NextEligible returns the first not-started task whose dependencies are all done.
func (idx *Index) NextEligible() *Task {
	statusMap := make(map[string]string, len(idx.tasks))
	for _, t := range idx.tasks {
		statusMap[t.ID] = t.Status
	}

	for i := range idx.tasks {
		t := &idx.tasks[i]
		if t.Status != "not-started" {
			continue
		}
		eligible := true
		for _, dep := range t.Dependencies {
			if statusMap[dep] != "done" {
				eligible = false
				break
			}
		}
		if eligible {
			out := *t
			return &out
		}
	}
	return nil
}

// Progress counts tasks by status.
func (idx *Index) Progress() TaskProgress {
	var p TaskProgress
	for _, t := range idx.tasks {
		switch t.Status {
		case "done":
			p.Done++
		case "in-progress":
			p.InProgress++
		case "not-started":
			p.NotStarted++
		case "blocked":
			p.Blocked++
		}
	}
	p.Total = len(idx.tasks)
	return p
}

// UpdateStatus changes a task's status in memory and rewrites the markdown file atomically.
func (idx *Index) UpdateStatus(taskID, status, reason string) error {
	if !validStatuses[status] {
		return fmt.Errorf("invalid status %q: must be one of not-started, in-progress, done, blocked", status)
	}

	found := false
	for i := range idx.tasks {
		if idx.tasks[i].ID == taskID {
			idx.tasks[i].Status = status
			if status == "blocked" {
				idx.tasks[i].BlockReason = reason
			} else {
				idx.tasks[i].BlockReason = ""
			}
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("task %q not found", taskID)
	}

	return idx.rewriteFile(taskID, status)
}

// rewriteFile re-reads the original file, patches the status cell for the
// given task ID, and writes back atomically via temp-file + rename.
func (idx *Index) rewriteFile(taskID, newStatus string) error {
	data, err := os.ReadFile(idx.filePath)
	if err != nil {
		return fmt.Errorf("reading index file for rewrite: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	headerCols := -1
	idCol, statusCol := -1, -1
	inTable := false
	patched := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			if inTable {
				// We've left the table region.
				break
			}
			continue
		}

		cells := splitTableRow(trimmed)

		// Detect header row (first table row with non-separator content).
		if !inTable {
			inTable = true
			headerCols = len(cells)
			for j, c := range cells {
				lower := strings.ToLower(strings.TrimSpace(c))
				switch lower {
				case "id":
					idCol = j
				case "status":
					statusCol = j
				}
			}
			continue
		}

		// Skip separator row.
		if isSeparatorRow(trimmed) {
			continue
		}

		if idCol < 0 || statusCol < 0 || idCol >= len(cells) || statusCol >= len(cells) {
			continue
		}

		if strings.TrimSpace(cells[idCol]) == taskID {
			cells[statusCol] = " " + newStatus + " "
			// Pad cells back to original column count.
			for len(cells) < headerCols {
				cells = append(cells, " ")
			}
			lines[i] = "|" + strings.Join(cells, "|") + "|"
			patched = true
			break
		}
	}

	if !patched {
		return fmt.Errorf("could not find task %q row in markdown table", taskID)
	}

	output := strings.Join(lines, "\n")
	return atomicWrite(idx.filePath, []byte(output))
}

// atomicWrite writes data to a temp file in the same directory then renames.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".task-index-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// parseTable extracts Task rows from a markdown table string.
func parseTable(content string, idPattern *regexp.Regexp) ([]Task, error) {
	lines := strings.Split(content, "\n")
	var tasks []Task

	idCol, titleCol, statusCol, depsCol := -1, -1, -1, -1
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			if headerFound {
				// Left the table.
				break
			}
			continue
		}

		if isSeparatorRow(trimmed) {
			continue
		}

		cells := splitTableRow(trimmed)

		if !headerFound {
			for j, c := range cells {
				lower := strings.ToLower(strings.TrimSpace(c))
				switch lower {
				case "id":
					idCol = j
				case "title":
					titleCol = j
				case "status":
					statusCol = j
				case "dependencies":
					depsCol = j
				}
			}
			if idCol < 0 || titleCol < 0 || statusCol < 0 {
				return nil, fmt.Errorf("table header must contain ID, Title, and Status columns")
			}
			headerFound = true
			continue
		}

		id := cellAt(cells, idCol)
		if id == "" {
			continue
		}
		if !idPattern.MatchString(id) {
			continue
		}

		status := strings.ToLower(cellAt(cells, statusCol))
		if !validStatuses[status] {
			status = "not-started"
		}

		var deps []string
		if depsCol >= 0 {
			raw := cellAt(cells, depsCol)
			if raw != "" {
				for _, d := range strings.Split(raw, ",") {
					d = strings.TrimSpace(d)
					if d != "" {
						deps = append(deps, d)
					}
				}
			}
		}

		tasks = append(tasks, Task{
			ID:           id,
			Title:        cellAt(cells, titleCol),
			Status:       status,
			Dependencies: deps,
		})
	}

	return tasks, nil
}

// splitTableRow splits a pipe-delimited row into cells,
// stripping the leading and trailing pipes.
func splitTableRow(row string) []string {
	row = strings.TrimSpace(row)
	row = strings.TrimPrefix(row, "|")
	row = strings.TrimSuffix(row, "|")
	parts := strings.Split(row, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// isSeparatorRow returns true for rows like |---|---|---|---|.
func isSeparatorRow(row string) bool {
	row = strings.TrimSpace(row)
	row = strings.TrimPrefix(row, "|")
	row = strings.TrimSuffix(row, "|")
	for _, cell := range strings.Split(row, "|") {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		cleaned := strings.ReplaceAll(cell, "-", "")
		cleaned = strings.ReplaceAll(cleaned, ":", "")
		if cleaned != "" {
			return false
		}
	}
	return true
}

// cellAt safely retrieves a trimmed cell value by index.
func cellAt(cells []string, col int) string {
	if col < 0 || col >= len(cells) {
		return ""
	}
	return strings.TrimSpace(cells[col])
}
