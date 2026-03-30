package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Config is the top-level ralph configuration.
type Config struct {
	Model      string       `yaml:"model"`
	Iterations int          `yaml:"iterations"`
	Prompt     string       `yaml:"prompt"`
	Tasks      TasksConfig  `yaml:"tasks"`
	Git        GitConfig    `yaml:"git"`
	Review     ReviewConfig `yaml:"review"`
	Logs       LogsConfig   `yaml:"logs"`
}

type TasksConfig struct {
	Directory string `yaml:"directory"`
	Index     string `yaml:"index"`
	IDPattern string `yaml:"id_pattern"`
}

type GitConfig struct {
	PushEvery int  `yaml:"push_every"`
	AutoStash bool `yaml:"auto_stash"`
	SkipPush  bool `yaml:"skip_push"`
}

type ReviewConfig struct {
	Enabled bool   `yaml:"enabled"`
	Prompt  string `yaml:"prompt"`
}

type LogsConfig struct {
	Directory     string `yaml:"directory"`
	RetentionDays int    `yaml:"retention_days"`
}

// DefaultConfig returns a Config with all default values filled in.
func DefaultConfig() *Config {
	return &Config{
		Model:      "",
		Iterations: 50,
		Prompt:     "eng/ralph.md",
		Tasks: TasksConfig{
			Directory: "tasks",
			Index:     "tasks/index.md",
			IDPattern: `T\d+`,
		},
		Git: GitConfig{
			PushEvery: 5,
			AutoStash: false,
			SkipPush:  false,
		},
		Review: ReviewConfig{
			Enabled: false,
			Prompt:  "",
		},
		Logs: LogsConfig{
			Directory:     "eng/logs",
			RetentionDays: 7,
		},
	}
}

// Load reads a YAML config file and merges it on top of defaults.
// If the file does not exist, defaults are returned without error.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}

	return cfg, nil
}

// Validate checks that the config values are sane.
func (c *Config) Validate() error {
	if c.Iterations <= 0 {
		return fmt.Errorf("iterations must be > 0, got %d", c.Iterations)
	}

	if _, err := regexp.Compile(c.Tasks.IDPattern); err != nil {
		return fmt.Errorf("invalid id_pattern %q: %w", c.Tasks.IDPattern, err)
	}

	if c.Logs.RetentionDays < 0 {
		return fmt.Errorf("retention_days must be >= 0, got %d", c.Logs.RetentionDays)
	}

	if c.Git.PushEvery < 0 {
		return fmt.Errorf("push_every must be >= 0, got %d", c.Git.PushEvery)
	}

	return nil
}

// FindConfigFile searches for eng/ralph.yaml starting from the current
// working directory and walking up to the filesystem root, similar to
// how git locates .git.
func FindConfigFile() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}

	for {
		candidate := filepath.Join(dir, "eng", "ralph.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("ralph.yaml not found (searched from %s upward)", dir)
		}
		dir = parent
	}
}
