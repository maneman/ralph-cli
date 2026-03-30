package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FindTaskSpec locates and reads a task spec markdown file for the given task ID.
// It searches tasksDir for files whose name starts with the task ID (case-insensitive).
func FindTaskSpec(tasksDir string, taskID string) (*TaskSpec, error) {
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return nil, fmt.Errorf("reading tasks directory %q: %w", tasksDir, err)
	}

	lowerID := strings.ToLower(taskID)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(name), lowerID) {
			continue
		}

		fullPath := filepath.Join(tasksDir, name)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("reading task spec %q: %w", fullPath, err)
		}

		title := titleFromFilename(name, taskID)

		return &TaskSpec{
			ID:      taskID,
			Title:   title,
			Content: string(data),
		}, nil
	}

	return nil, fmt.Errorf("no spec file found for task %q in %q", taskID, tasksDir)
}

// titleFromFilename derives a human-readable title from a spec filename.
// E.g. "T001-set-up-auth-module.md" → "Set up auth module"
func titleFromFilename(filename, taskID string) string {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Strip the task ID prefix and any separator.
	lower := strings.ToLower(name)
	lowerID := strings.ToLower(taskID)
	if strings.HasPrefix(lower, lowerID) {
		name = name[len(taskID):]
	}
	name = strings.TrimLeft(name, "-_ ")

	if name == "" {
		return taskID
	}

	// Replace hyphens/underscores with spaces.
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.TrimSpace(name)

	if name == "" {
		return taskID
	}

	// Capitalize first letter.
	return strings.ToUpper(name[:1]) + name[1:]
}
