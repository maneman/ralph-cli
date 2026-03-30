package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mane/ralph-cli/prompts"
)

func TestBuild_CoreOnly(t *testing.T) {
	result, err := Build("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != prompts.CorePrompt {
		t.Errorf("expected core prompt only, got length %d (core is %d)", len(result), len(prompts.CorePrompt))
	}
}

func TestBuild_EmptyPathNoGap(t *testing.T) {
	result, err := Build("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Ralph") {
		t.Error("expected core prompt to mention Ralph")
	}
}

func TestBuild_WithProjectPrompt(t *testing.T) {
	dir := t.TempDir()
	pp := filepath.Join(dir, "project.md")
	if err := os.WriteFile(pp, []byte("# Project Instructions\nUse Go."), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Build(pp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result, prompts.CorePrompt) {
		t.Error("result should start with core prompt")
	}
	if !strings.Contains(result, "---") {
		t.Error("expected separator between core and project prompt")
	}
	if !strings.Contains(result, "# Project Instructions") {
		t.Error("expected project prompt content")
	}
}

func TestBuild_WithGapContext(t *testing.T) {
	gap := &GapContext{
		TaskID:      "task-42",
		Attempt:     2,
		MaxAttempts: 3,
		GapDetails:  "- Missing error handling in parser\n- No unit tests",
	}

	result, err := Build("", gap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "GAP-FILL MODE") {
		t.Error("expected GAP-FILL MODE header")
	}
	if !strings.Contains(result, "task-42") {
		t.Error("expected task ID in output")
	}
	if !strings.Contains(result, "attempt 2/3") {
		t.Error("expected attempt counts")
	}
	if !strings.Contains(result, "Missing error handling") {
		t.Error("expected gap details")
	}
}

func TestBuild_WithProjectAndGap(t *testing.T) {
	dir := t.TempDir()
	pp := filepath.Join(dir, "project.md")
	if err := os.WriteFile(pp, []byte("# My Project"), 0644); err != nil {
		t.Fatal(err)
	}

	gap := &GapContext{
		TaskID:      "fix-bug",
		Attempt:     1,
		MaxAttempts: 2,
		GapDetails:  "- Tests still failing",
	}

	result, err := Build(pp, gap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coreIdx := strings.Index(result, prompts.CorePrompt)
	projectIdx := strings.Index(result, "# My Project")
	gapIdx := strings.Index(result, "GAP-FILL MODE")

	if coreIdx < 0 || projectIdx < 0 || gapIdx < 0 {
		t.Fatal("expected all three sections present")
	}
	if coreIdx >= projectIdx {
		t.Error("core prompt should come before project prompt")
	}
	if projectIdx >= gapIdx {
		t.Error("project prompt should come before gap context")
	}
}

func TestBuild_MissingProjectFile(t *testing.T) {
	_, err := Build("/nonexistent/path/to/prompt.md", nil)
	if err == nil {
		t.Fatal("expected error for missing project prompt file")
	}
	if !strings.Contains(err.Error(), "reading project prompt") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}
