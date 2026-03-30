package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mane/ralph-cli/internal/agent"
	"github.com/mane/ralph-cli/internal/config"
	gitpkg "github.com/mane/ralph-cli/internal/git"
	"github.com/mane/ralph-cli/internal/prompt"
	"github.com/mane/ralph-cli/internal/tasks"
	"github.com/mane/ralph-cli/internal/telemetry"
	"github.com/mane/ralph-cli/internal/tui"
)

// Engine is the main loop orchestrator that ties all ralph-cli subsystems
// together.
type Engine struct {
	cfg       *config.Config
	state     *State
	taskIndex *tasks.Index
	git       *gitpkg.Git
	pusher    *gitpkg.BatchPusher
	logger    *telemetry.Logger
	output    tui.Output
	statePath string
	workDir   string
	tasksDir  string
}

// Options configures a new Engine.
type Options struct {
	Config  *config.Config
	Output  tui.Output
	WorkDir string
}

// RunReport is returned by Engine.Run with aggregated results.
type RunReport struct {
	Summary  telemetry.RunSummary
	Baseline *telemetry.BaselineComparison
}

// New initialises the engine: loads persisted state, parses the task index,
// and wires up git / telemetry / TUI.
func New(opts Options) (*Engine, error) {
	cfg := opts.Config
	workDir := opts.WorkDir

	statePath := filepath.Join(workDir, ".ralph-state.json")
	state, err := LoadState(statePath)
	if err != nil {
		return nil, fmt.Errorf("loading state: %w", err)
	}

	indexPath := filepath.Join(workDir, cfg.Tasks.Index)
	taskIndex, err := tasks.LoadIndex(indexPath, cfg.Tasks.IDPattern)
	if err != nil {
		return nil, fmt.Errorf("loading task index: %w", err)
	}

	g := gitpkg.New(workDir)
	pusher := gitpkg.NewBatchPusher(g, cfg.Git.PushEvery, cfg.Git.SkipPush)

	logsDir := filepath.Join(workDir, cfg.Logs.Directory)
	logger, err := telemetry.NewLogger(logsDir, cfg.Logs.RetentionDays)
	if err != nil {
		return nil, fmt.Errorf("creating logger: %w", err)
	}

	tasksDir := filepath.Join(workDir, cfg.Tasks.Directory)

	return &Engine{
		cfg:       cfg,
		state:     state,
		taskIndex: taskIndex,
		git:       g,
		pusher:    pusher,
		logger:    logger,
		output:    opts.Output,
		statePath: statePath,
		workDir:   workDir,
		tasksDir:  tasksDir,
	}, nil
}

// Close releases resources held by the engine (e.g. log file handles).
func (e *Engine) Close() error {
	return e.logger.Close()
}

// Run executes the main iteration loop and returns an aggregate report.
func (e *Engine) Run(ctx context.Context) (*RunReport, error) {
	startTime := time.Now()

	var (
		successes    int
		gapsFound    int
		noOps        int
		crashes      int
		blockedCount int
		totalTokens  int
		totalCost    float64
		prdComplete  bool
		completed    int
		totalRetries int
	)

	for i := 1; i <= e.cfg.Iterations; i++ {
		if ctx.Err() != nil {
			break
		}

		e.state.Iteration = i

		// Check if all tasks are done before starting the iteration.
		progress := e.taskIndex.Progress()
		if progress.NotStarted == 0 && progress.InProgress == 0 {
			prdComplete = true
			break
		}

		completed++
		iterStart := time.Now()

		// --- 1. Update TUI header ---
		e.output.Send(tui.HeaderUpdateMsg{Header: tui.HeaderState{
			Iteration:     i,
			TotalIter:     e.cfg.Iterations,
			TaskID:        e.state.TaskID,
			TotalDuration: time.Since(startTime),
			Done:          progress.Done,
			Remaining:     progress.NotStarted,
			Retries:       e.state.Attempt,
			Blocked:       progress.Blocked,
			Model:         e.cfg.Model,
			Phase:         "running",
		}})

		// --- 2. Build prompt (with optional gap context) ---
		var gapCtx *prompt.GapContext
		if e.state.Mode == "gap-fill" {
			gapCtx = &prompt.GapContext{
				TaskID:      e.state.TaskID,
				Attempt:     e.state.Attempt,
				MaxAttempts: e.state.MaxAttempts,
				GapDetails:  e.state.GapDetails,
			}
		}

		fullPrompt, err := prompt.Build(e.cfg.Prompt, gapCtx)
		if err != nil {
			return nil, fmt.Errorf("building prompt: %w", err)
		}

		// --- 3. Set up ToolCallbacks ---
		var agentOutput strings.Builder
		iterTaskID := e.state.TaskID
		callbacks := e.buildCallbacks(&iterTaskID)

		// --- 4. Record git HEAD ref ---
		headRef, _ := e.git.HeadRef()

		// --- 5. Run agent session ---
		result, runErr := agent.Run(ctx, agent.SessionConfig{
			Model:       e.cfg.Model,
			Prompt:      fullPrompt,
			Callbacks:   callbacks,
			OnEvent:     e.eventForwarder(&agentOutput),
			AutoApprove: true,
		})

		// --- 6. Count new commits ---
		var commitCount int
		if headRef != "" {
			commitCount, _ = e.git.CommitsSince(headRef)
		}

		// --- 7. Classify outcome ---
		outcome := ClassifyOutcome(result, commitCount, e.state, runErr)

		// Track tokens & cost from result.
		if result != nil {
			totalTokens += result.Tokens
			totalCost += result.Cost
			if iterTaskID == "" {
				iterTaskID = result.TaskID
			}
		}

		iterDuration := time.Since(iterStart)

		// --- 8. Route by outcome ---
		switch outcome {
		case OutcomeSuccess:
			successes++
			e.state.RecordOutcome(OutcomeSuccess)
			e.state.Reset()

		case OutcomeGapsFound:
			gapsFound++
			e.state.Attempt++
			totalRetries++
			if e.state.Attempt < e.state.MaxAttempts {
				e.state.Mode = "gap-fill"
				if result != nil {
					e.state.GapDetails = result.Summary
				}
			} else {
				if e.state.TaskID != "" {
					_ = e.taskIndex.UpdateStatus(e.state.TaskID, "blocked", "max gap-fill attempts reached")
					e.state.BlockedTasks = append(e.state.BlockedTasks, BlockedTask{
						ID:     e.state.TaskID,
						Reason: "max gap-fill attempts reached",
						Gaps:   e.state.GapDetails,
					})
					blockedCount++
				}
				e.state.Reset()
			}
			e.state.RecordOutcome(OutcomeGapsFound)

		case OutcomeNoOp:
			noOps++
			e.state.RecordOutcome(OutcomeNoOp)
			if e.state.IsStuck() {
				outcome = OutcomeStuck
				if e.state.TaskID != "" {
					_ = e.taskIndex.UpdateStatus(e.state.TaskID, "blocked", "stuck: consecutive no-ops")
					e.state.BlockedTasks = append(e.state.BlockedTasks, BlockedTask{
						ID:     e.state.TaskID,
						Reason: "stuck: consecutive no-ops",
					})
					blockedCount++
				}
				e.state.Reset()
			}

		case OutcomeAgentCrash:
			crashes++
			e.state.Attempt++
			totalRetries++
			e.state.RecordOutcome(OutcomeAgentCrash)
			if e.state.Attempt >= e.state.MaxAttempts {
				if e.state.TaskID != "" {
					_ = e.taskIndex.UpdateStatus(e.state.TaskID, "blocked", "max retry attempts after crashes")
					e.state.BlockedTasks = append(e.state.BlockedTasks, BlockedTask{
						ID:     e.state.TaskID,
						Reason: "max retry attempts after crashes",
					})
					blockedCount++
				}
				e.state.Reset()
			}

		case OutcomeStuck:
			blockedCount++
			e.state.RecordOutcome(OutcomeStuck)
			if e.state.TaskID != "" {
				_ = e.taskIndex.UpdateStatus(e.state.TaskID, "blocked", "agent reported stuck/blocked")
				e.state.BlockedTasks = append(e.state.BlockedTasks, BlockedTask{
					ID:     e.state.TaskID,
					Reason: "agent reported stuck/blocked",
				})
			}
			e.state.Reset()

		case OutcomePRDComplete:
			prdComplete = true
			e.state.RecordOutcome(OutcomePRDComplete)
		}

		// --- 9. Save state ---
		_ = e.state.Save(e.statePath)

		// --- 10. Log telemetry ---
		var tokens int
		var cost float64
		if result != nil {
			tokens = result.Tokens
			cost = result.Cost
		}
		_ = e.logger.LogOutcome(telemetry.IterationOutcome{
			Iteration: i,
			TaskID:    iterTaskID,
			Outcome:   outcome,
			Duration:  iterDuration,
			Attempt:   e.state.Attempt,
			Commits:   commitCount,
			Tokens:    tokens,
			Cost:      cost,
			Timestamp: time.Now(),
		})

		// --- 11. Write full agent output for this iteration ---
		_ = e.logger.WriteIterationLog(i, agentOutput.String())

		// --- 12. Batch push ---
		_, _ = e.pusher.AfterIteration()

		// --- 13. Iteration separator in TUI ---
		e.output.Send(tui.IterationSeparatorMsg{
			Iteration: i,
			TaskID:    iterTaskID,
			Duration:  iterDuration,
			Outcome:   outcome,
		})

		if prdComplete {
			break
		}
	}

	// ---- Post-loop ----

	// Flush remaining pushes.
	_ = e.pusher.Flush()

	endTime := time.Now()
	totalTime := endTime.Sub(startTime)

	summary := telemetry.RunSummary{
		StartTime:   startTime,
		EndTime:     endTime,
		TotalTime:   totalTime,
		Iterations:  e.cfg.Iterations,
		Completed:   completed,
		Successes:   successes,
		GapsFound:   gapsFound,
		NoOps:       noOps,
		Crashes:     crashes,
		Blocked:     blockedCount,
		PRDComplete: prdComplete,
		TotalTokens: totalTokens,
		TotalCost:   totalCost,
	}
	if completed > 0 {
		summary.SuccessRate = float64(successes) / float64(completed)
		summary.GapRate = float64(gapsFound) / float64(completed)
	}
	attempts := successes + gapsFound + crashes
	if attempts > 0 {
		summary.AvgRetries = float64(totalRetries) / float64(attempts)
	}

	// Compute baseline comparison against past runs.
	runsFile := filepath.Join(e.workDir, e.cfg.Logs.Directory, "ralph-runs.jsonl")
	baseline := telemetry.ComputeBaseline(summary, runsFile)

	// Persist run summary.
	_ = e.logger.LogRun(summary)

	// Send summary to TUI.
	e.output.Send(tui.RunSummaryMsg{
		Iterations:  completed,
		Successes:   successes,
		GapsFound:   gapsFound,
		Blocked:     blockedCount,
		SuccessRate: summary.SuccessRate,
		TotalTime:   totalTime,
		TotalTokens: totalTokens,
		TotalCost:   totalCost,
	})

	return &RunReport{
		Summary:  summary,
		Baseline: baseline,
	}, nil
}

// buildCallbacks wires the agent tool callbacks to the engine's task index
// and state. iterTaskID is written when the agent picks a task so the caller
// can track which task was worked on.
func (e *Engine) buildCallbacks(iterTaskID *string) *agent.ToolCallbacks {
	return &agent.ToolCallbacks{
		ListTasks: func() ([]tasks.Task, error) {
			return e.taskIndex.ListTasks(), nil
		},
		PickTask: func() (*agent.PickTaskResult, error) {
			var t *tasks.Task
			if e.state.Mode == "gap-fill" && e.state.TaskID != "" {
				t = e.taskIndex.FindTask(e.state.TaskID)
			} else {
				t = e.taskIndex.NextEligible()
			}
			if t == nil {
				return nil, fmt.Errorf("no eligible tasks available")
			}

			spec, err := tasks.FindTaskSpec(e.tasksDir, t.ID)
			if err != nil {
				return nil, err
			}

			e.state.TaskID = t.ID
			*iterTaskID = t.ID

			return &agent.PickTaskResult{
				ID:          t.ID,
				Title:       t.Title,
				SpecContent: spec.Content,
				Attempt:     e.state.Attempt,
				GapContext:  e.state.GapDetails,
			}, nil
		},
		GetTaskSpec: func(taskID string) (*tasks.TaskSpec, error) {
			return tasks.FindTaskSpec(e.tasksDir, taskID)
		},
		UpdateStatus: func(taskID, status, reason string) error {
			return e.taskIndex.UpdateStatus(taskID, status, reason)
		},
		ReportOutcome: func(taskID, outcome, summary string) error {
			return nil
		},
	}
}

// eventForwarder returns an event handler that writes assistant deltas into
// output and forwards every event to the TUI.
func (e *Engine) eventForwarder(output *strings.Builder) func(agent.Event) {
	return func(ev agent.Event) {
		switch ev.Type {
		case agent.EventAssistantDelta:
			output.WriteString(ev.Content)
			e.output.Send(tui.AgentOutputMsg{Content: ev.Content})
		case agent.EventToolStart:
			e.output.Send(tui.ToolStartMsg{
				ToolName:  ev.ToolName,
				Arguments: ev.Arguments,
			})
		case agent.EventToolPartialOutput:
			e.output.Send(tui.ToolOutputMsg{
				ToolCallID: ev.ToolCallID,
				Output:     ev.Content,
			})
		case agent.EventToolComplete:
			e.output.Send(tui.ToolCompleteMsg{
				ToolCallID: ev.ToolCallID,
				ToolName:   ev.ToolName,
				Success:    ev.Success,
				Duration:   ev.Duration,
			})
		}
	}
}
