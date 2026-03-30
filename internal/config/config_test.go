package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Iterations != 50 {
		t.Errorf("Iterations = %d, want 50", cfg.Iterations)
	}
	if cfg.Prompt != "eng/ralph.md" {
		t.Errorf("Prompt = %q, want %q", cfg.Prompt, "eng/ralph.md")
	}
	if cfg.Tasks.Directory != "tasks" {
		t.Errorf("Tasks.Directory = %q, want %q", cfg.Tasks.Directory, "tasks")
	}
	if cfg.Tasks.Index != "tasks/index.md" {
		t.Errorf("Tasks.Index = %q, want %q", cfg.Tasks.Index, "tasks/index.md")
	}
	if cfg.Tasks.IDPattern != `T\d+` {
		t.Errorf("Tasks.IDPattern = %q, want %q", cfg.Tasks.IDPattern, `T\d+`)
	}
	if cfg.Git.PushEvery != 5 {
		t.Errorf("Git.PushEvery = %d, want 5", cfg.Git.PushEvery)
	}
	if cfg.Git.AutoStash {
		t.Error("Git.AutoStash should be false")
	}
	if cfg.Git.SkipPush {
		t.Error("Git.SkipPush should be false")
	}
	if cfg.Review.Enabled {
		t.Error("Review.Enabled should be false")
	}
	if cfg.Logs.Directory != "eng/logs" {
		t.Errorf("Logs.Directory = %q, want %q", cfg.Logs.Directory, "eng/logs")
	}
	if cfg.Logs.RetentionDays != 7 {
		t.Errorf("Logs.RetentionDays = %d, want 7", cfg.Logs.RetentionDays)
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig().Validate() = %v, want nil", err)
	}
}

func TestLoadValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ralph.yaml")
	data := []byte(`
model: "gpt-4"
iterations: 100
prompt: "custom/prompt.md"
tasks:
  directory: "my-tasks"
  index: "my-tasks/idx.md"
  id_pattern: "TASK-\\d+"
git:
  push_every: 10
  auto_stash: true
  skip_push: true
review:
  enabled: true
  prompt: "review.md"
logs:
  directory: "out/logs"
  retention_days: 14
`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", cfg.Model, "gpt-4")
	}
	if cfg.Iterations != 100 {
		t.Errorf("Iterations = %d, want 100", cfg.Iterations)
	}
	if cfg.Prompt != "custom/prompt.md" {
		t.Errorf("Prompt = %q, want %q", cfg.Prompt, "custom/prompt.md")
	}
	if cfg.Tasks.Directory != "my-tasks" {
		t.Errorf("Tasks.Directory = %q, want %q", cfg.Tasks.Directory, "my-tasks")
	}
	if cfg.Tasks.IDPattern != `TASK-\d+` {
		t.Errorf("Tasks.IDPattern = %q, want %q", cfg.Tasks.IDPattern, `TASK-\d+`)
	}
	if cfg.Git.PushEvery != 10 {
		t.Errorf("Git.PushEvery = %d, want 10", cfg.Git.PushEvery)
	}
	if !cfg.Git.AutoStash {
		t.Error("Git.AutoStash should be true")
	}
	if !cfg.Git.SkipPush {
		t.Error("Git.SkipPush should be true")
	}
	if !cfg.Review.Enabled {
		t.Error("Review.Enabled should be true")
	}
	if cfg.Review.Prompt != "review.md" {
		t.Errorf("Review.Prompt = %q, want %q", cfg.Review.Prompt, "review.md")
	}
	if cfg.Logs.Directory != "out/logs" {
		t.Errorf("Logs.Directory = %q, want %q", cfg.Logs.Directory, "out/logs")
	}
	if cfg.Logs.RetentionDays != 14 {
		t.Errorf("Logs.RetentionDays = %d, want 14", cfg.Logs.RetentionDays)
	}
}

func TestLoadPartialYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ralph.yaml")
	data := []byte(`
model: "claude-sonnet"
git:
  auto_stash: true
`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Model != "claude-sonnet" {
		t.Errorf("Model = %q, want %q", cfg.Model, "claude-sonnet")
	}
	if !cfg.Git.AutoStash {
		t.Error("Git.AutoStash should be true")
	}
	// Defaults preserved for unset fields.
	if cfg.Iterations != 50 {
		t.Errorf("Iterations = %d, want 50 (default)", cfg.Iterations)
	}
	if cfg.Tasks.Directory != "tasks" {
		t.Errorf("Tasks.Directory = %q, want %q (default)", cfg.Tasks.Directory, "tasks")
	}
	if cfg.Logs.RetentionDays != 7 {
		t.Errorf("Logs.RetentionDays = %d, want 7 (default)", cfg.Logs.RetentionDays)
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	cfg, err := Load("/no/such/file.yaml")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for missing file", err)
	}
	if cfg.Iterations != 50 {
		t.Errorf("Iterations = %d, want 50 (default)", cfg.Iterations)
	}
}

func TestValidateInvalidIterations(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Iterations = 0
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for iterations=0")
	}
}

func TestValidateBadRegex(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tasks.IDPattern = "[invalid"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for bad id_pattern regex")
	}
}

func TestValidateNegativeRetention(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logs.RetentionDays = -1
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for negative retention_days")
	}
}

func TestValidateNegativePushEvery(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Git.PushEvery = -1
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for negative push_every")
	}
}

func TestFindConfigFile(t *testing.T) {
	// Create a temp directory tree: root/eng/ralph.yaml and root/sub/deep/
	root := t.TempDir()
	engDir := filepath.Join(root, "eng")
	if err := os.MkdirAll(engDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(engDir, "ralph.yaml")
	if err := os.WriteFile(cfgPath, []byte("model: test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	deepDir := filepath.Join(root, "sub", "deep")
	if err := os.MkdirAll(deepDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Change into the deep directory and search upward.
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	if err := os.Chdir(deepDir); err != nil {
		t.Fatal(err)
	}

	found, err := FindConfigFile()
	if err != nil {
		t.Fatalf("FindConfigFile() error = %v", err)
	}
	if found != cfgPath {
		t.Errorf("FindConfigFile() = %q, want %q", found, cfgPath)
	}
}

func TestFindConfigFileNotFound(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	_, err := FindConfigFile()
	if err == nil {
		t.Error("FindConfigFile() should fail when no config exists")
	}
}
