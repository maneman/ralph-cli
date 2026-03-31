package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mane/ralph-cli/internal/config"
	"github.com/mane/ralph-cli/internal/engine"
	"github.com/mane/ralph-cli/internal/tasks"
	"github.com/mane/ralph-cli/internal/tui"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     "ralph",
		Short:   "Autonomous task completion loop powered by GitHub Copilot",
		Long:    "Ralph is a CLI tool that wraps the GitHub Copilot SDK to run an autonomous task completion loop with a polished TUI.",
		Version: version,
		RunE:    executeRun,
		// Silence cobra's default error/usage printing so we control output.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start the autonomous task completion loop",
		RunE:  executeRun,
	}

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold project files for Ralph",
		RunE:  executeInit,
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show current Ralph state",
		RunE:  executeStatus,
	}

	rootCmd.AddCommand(runCmd, initCmd, statusCmd)

	// Both root and run accept the same flags so bare `ralph` works like `ralph run`.
	addRunFlags(rootCmd)
	addRunFlags(runCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// Flags
// ---------------------------------------------------------------------------

func addRunFlags(cmd *cobra.Command) {
	cmd.Flags().Int("iterations", 0, "Maximum number of iterations (default from config or 50)")
	cmd.Flags().String("model", "", "Model to use for completions")
	cmd.Flags().Bool("review", false, "Enable review mode")
	cmd.Flags().Int("push-every", 0, "Push every N iterations (default from config or 5)")
	cmd.Flags().Bool("auto-stash", false, "Automatically stash changes before pulling")
	cmd.Flags().Bool("skip-push", false, "Skip pushing changes")
	cmd.Flags().Bool("no-tui", false, "Disable TUI and use plain text output")
	cmd.Flags().String("config", "", "Explicit config file path")
}

// ---------------------------------------------------------------------------
// Config helper
// ---------------------------------------------------------------------------

func loadConfig(explicitPath string) (*config.Config, error) {
	if explicitPath != "" {
		return config.Load(explicitPath)
	}
	cfgPath, err := config.FindConfigFile()
	if err != nil {
		// No config file found; use defaults.
		return config.DefaultConfig(), nil
	}
	return config.Load(cfgPath)
}

// ---------------------------------------------------------------------------
// run
// ---------------------------------------------------------------------------

func executeRun(cmd *cobra.Command, _ []string) error {
	// 1. Load config, merge with CLI flags.
	configPath, _ := cmd.Flags().GetString("config")
	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if cmd.Flags().Changed("iterations") {
		cfg.Iterations, _ = cmd.Flags().GetInt("iterations")
	}
	if cmd.Flags().Changed("model") {
		cfg.Model, _ = cmd.Flags().GetString("model")
	}
	if cmd.Flags().Changed("review") {
		cfg.Review.Enabled, _ = cmd.Flags().GetBool("review")
	}
	if cmd.Flags().Changed("push-every") {
		cfg.Git.PushEvery, _ = cmd.Flags().GetInt("push-every")
	}
	if cmd.Flags().Changed("auto-stash") {
		cfg.Git.AutoStash, _ = cmd.Flags().GetBool("auto-stash")
	}
	if cmd.Flags().Changed("skip-push") {
		cfg.Git.SkipPush, _ = cmd.Flags().GetBool("skip-push")
	}

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// 2. Acquire singleton lock.
	lockPath := filepath.Join(workDir, ".ralph.lock")
	if err := engine.AcquireLock(lockPath); err != nil {
		return fmt.Errorf("lock: %w", err)
	}

	// 3. Create output.
	noTUI, _ := cmd.Flags().GetBool("no-tui")
	var output tui.Output
	if noTUI {
		output = tui.NewPlainOutput()
	} else {
		output = tui.NewTUIOutput()
	}
	if err := output.Start(); err != nil {
		_ = engine.ReleaseLock(lockPath)
		return fmt.Errorf("starting output: %w", err)
	}

	// 4. Create engine.
	eng, err := engine.New(engine.Options{
		Config:  cfg,
		Output:  output,
		WorkDir: workDir,
	})
	if err != nil {
		output.Shutdown()
		_ = engine.ReleaseLock(lockPath)
		return fmt.Errorf("creating engine: %w", err)
	}

	// 5. Signal handling → context cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nReceived signal, shutting down gracefully...")
		cancel()
		// A second signal forces immediate exit.
		<-sigCh
		fmt.Fprintln(os.Stderr, "Force quit.")
		os.Exit(1)
	}()

	// 6. Run the engine.
	report, runErr := eng.Run(ctx)

	// 7. Tear down in order: TUI → engine → lock → signals.
	cancel()
	output.Shutdown()
	output.Wait()
	_ = eng.Close()
	_ = engine.ReleaseLock(lockPath)
	signal.Stop(sigCh)

	if runErr != nil {
		return fmt.Errorf("engine run failed: %w", runErr)
	}

	// 8. Print report after TUI has exited.
	if report != nil {
		printRunReport(report)
	}

	return nil
}

func printRunReport(r *engine.RunReport) {
	s := r.Summary
	fmt.Fprintf(os.Stderr, "\n══ Run Complete ══\n")
	fmt.Fprintf(os.Stderr, "  Iterations: %d/%d completed\n", s.Completed, s.Iterations)
	fmt.Fprintf(os.Stderr, "  Successes:  %d (%.1f%%)\n", s.Successes, s.SuccessRate*100)
	fmt.Fprintf(os.Stderr, "  Gaps Found: %d\n", s.GapsFound)
	fmt.Fprintf(os.Stderr, "  Blocked:    %d\n", s.Blocked)
	fmt.Fprintf(os.Stderr, "  Total Time: %s\n", s.TotalTime.Truncate(time.Second))
	if s.PRDComplete {
		fmt.Fprintln(os.Stderr, "  ✓ All tasks complete!")
	}

	if b := r.Baseline; b != nil {
		fmt.Fprintf(os.Stderr, "\n── Baseline Comparison ──\n")
		fmt.Fprintf(os.Stderr, "  Success Rate: %.1f%% %s (%+.1f%%)\n",
			b.SuccessRate.Current*100, b.SuccessRate.Symbol, b.SuccessRate.Diff*100)
		fmt.Fprintf(os.Stderr, "  Gap Rate:     %.1f%% %s (%+.1f%%)\n",
			b.GapRate.Current*100, b.GapRate.Symbol, b.GapRate.Diff*100)
		fmt.Fprintf(os.Stderr, "  Avg Retries:  %.1f  %s (%+.1f)\n",
			b.AvgRetries.Current, b.AvgRetries.Symbol, b.AvgRetries.Diff)
	}
}

// ---------------------------------------------------------------------------
// init
// ---------------------------------------------------------------------------

func executeInit(_ *cobra.Command, _ []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	var created, skipped int

	write := func(rel, content string) error {
		p := filepath.Join(workDir, rel)
		if _, err := os.Stat(p); err == nil {
			fmt.Fprintf(os.Stderr, "  exists: %s (skipped)\n", rel)
			skipped++
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "  created: %s\n", rel)
		created++
		return nil
	}

	fmt.Fprintln(os.Stderr, "Initialising Ralph project...")

	if err := write("eng/ralph.yaml", scaffoldConfigYAML); err != nil {
		return err
	}
	if err := write("eng/ralph.md", scaffoldPromptMD); err != nil {
		return err
	}
	if err := write("tasks/index.md", scaffoldTaskIndex); err != nil {
		return err
	}
	if err := write("eng/logs/.gitkeep", ""); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\nDone — %d created, %d skipped.\n", created, skipped)
	if created > 0 {
		fmt.Fprintln(os.Stderr, "\nNext steps:")
		fmt.Fprintln(os.Stderr, "  1. Edit eng/ralph.yaml with your preferred model and settings")
		fmt.Fprintln(os.Stderr, "  2. Edit eng/ralph.md with your project-specific instructions")
		fmt.Fprintln(os.Stderr, "  3. Add tasks to tasks/index.md")
		fmt.Fprintln(os.Stderr, "  4. Run `ralph` to start the autonomous loop")
	}

	return nil
}

// ---------------------------------------------------------------------------
// status
// ---------------------------------------------------------------------------

func executeStatus(_ *cobra.Command, _ []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Load state (the engine stores it at .ralph-state.json in workDir).
	statePath := filepath.Join(workDir, ".ralph-state.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		fmt.Println("No ralph state found. Run `ralph init` to set up.")
		return nil
	}
	state, err := engine.LoadState(statePath)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	// Load config for task index settings.
	cfg, _ := loadConfig("")

	fmt.Println("══ Ralph Status ══")
	fmt.Printf("  Mode:       %s\n", state.Mode)
	if state.TaskID != "" {
		fmt.Printf("  Task:       %s\n", state.TaskID)
	}
	fmt.Printf("  Attempt:    %d / %d\n", state.Attempt, state.MaxAttempts)
	fmt.Printf("  Iteration:  %d\n", state.Iteration)

	// Load task index (best-effort).
	indexPath := filepath.Join(workDir, cfg.Tasks.Index)
	idx, idxErr := tasks.LoadIndex(indexPath, cfg.Tasks.IDPattern)
	if idxErr == nil {
		p := idx.Progress()
		fmt.Printf("\n── Task Progress ──\n")
		fmt.Printf("  Done:        %d\n", p.Done)
		fmt.Printf("  In Progress: %d\n", p.InProgress)
		fmt.Printf("  Not Started: %d\n", p.NotStarted)
		fmt.Printf("  Blocked:     %d\n", p.Blocked)
		fmt.Printf("  Total:       %d\n", p.Total)
	}

	if len(state.BlockedTasks) > 0 {
		fmt.Printf("\n── Blocked Tasks ──\n")
		for _, bt := range state.BlockedTasks {
			fmt.Printf("  %s — %s\n", bt.ID, bt.Reason)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Scaffold templates
// ---------------------------------------------------------------------------

const scaffoldConfigYAML = `# Ralph configuration
# See docs for all options.

# Model to use for agent completions (e.g. "gpt-4o", "claude-sonnet-4-20250514")
# model: ""

# Maximum iterations per run
iterations: 50

# Path to the project prompt file
prompt: eng/ralph.md

# Task management
tasks:
  directory: tasks
  index: tasks/index.md
  id_pattern: 'T\d+'

# Git settings
git:
  push_every: 5
  auto_stash: false
  skip_push: false

# Review settings
review:
  enabled: false
  # prompt: ""

# Logging
logs:
  directory: eng/logs
  retention_days: 7
`

const scaffoldPromptMD = `# Project-Specific Instructions

## Build & Test
<!-- Replace with your project's commands -->
- Build: ` + "`go build ./...`" + `
- Test: ` + "`go test ./...`" + `
- Lint: ` + "`golangci-lint run`" + `

## Code Conventions
<!-- Describe your project's coding standards -->
- Follow existing patterns in the codebase
- Write tests for all new functionality

## Architecture Notes
<!-- Key architectural decisions and constraints -->
`

const scaffoldTaskIndex = `# Task Index

| ID | Title | Status | Dependencies |
|----|-------|--------|--------------|
| T001 | Example task | not-started | |
`
