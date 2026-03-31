# Copilot Instructions for ralph-cli

## Project Overview

Ralph-cli is a Go CLI tool that runs GitHub Copilot in an autonomous loop to complete tasks from a markdown backlog. It uses the Copilot SDK (`github.com/github/copilot-sdk/go`) for agent sessions, registers 5 custom tools for structured task management, and renders a polished Bubbletea TUI.

## Architecture

```
cmd/ralph/main.go          CLI entrypoint (cobra subcommands: run, init, status)
internal/
  config/                  YAML config loading (eng/ralph.yaml) with defaults + validation
  engine/                  Main loop orchestrator, state machine, outcome routing, lock
  agent/                   Copilot SDK integration, 5 custom tools, event streaming
  tasks/                   Markdown task index parser, status updates, spec file lookup
  git/                     Git operations (commit detection, batch push, auto-stash)
  tui/                     Bubbletea split-pane TUI + plain text fallback
  telemetry/               JSONL logging, run summaries, baseline comparison
  prompt/                  Core prompt embedding + project prompt concatenation
prompts/
  core.md                  Built-in agent prompt (7 phases, invariants, tool docs)
  embed.go                 go:embed for core.md
npm/                       npm wrapper package (esbuild-style binary distribution)
```

## Coding Conventions

### Go Style
- **Error handling:** Always return `error`, wrap with `fmt.Errorf("context: %w", err)`. Never panic except at startup.
- **Naming:** PascalCase for exported types/functions, camelCase for unexported. Constants use PascalCase with prefix (e.g., `OutcomeSuccess`, `EventToolStart`).
- **Imports:** Standard library first, then external packages, then internal packages. Blank line between groups.
- **File organization:** One logical type per file. Receiver methods follow the type definition. Helpers at bottom.
- **Concurrency:** Use `sync.Mutex` for shared state, `sync.Once` for one-time operations, channels for signaling. Context propagation for cancellation.

### Testing
- Tests live in `*_test.go` in the same package (not `_test` suffix packages).
- Use table-driven tests with subtests (`t.Run(name, func(t *testing.T) {...})`).
- Use `t.TempDir()` for file I/O tests, `t.Helper()` for test helpers.
- Standard library `testing` only — no external test frameworks.
- Assertions via `if got != want { t.Errorf(...) }` or `t.Fatalf(...)` for fatal errors.

### Resource Management
- Atomic file writes: write to temp file, then `os.Rename()` (see `tasks/index.go`).
- Always `defer` cleanup: `defer f.Close()`, `defer client.Stop()`.
- File handles stored in structs must have a `Close()` method.

## Key Design Decisions

### Custom SDK Tools (not XML parsing)
The agent communicates with ralph through 5 registered tools — never by editing files or emitting XML tags:
- `ralph_list_tasks` — returns all tasks as JSON
- `ralph_pick_task` — returns next eligible task with spec content
- `ralph_get_task_spec` — reads a specific task's markdown spec
- `ralph_update_status` — atomically updates task status in the index
- `ralph_report_outcome` — signals iteration completion (success/stuck/blocked)

Tools are registered via `copilot.DefineTool()` with typed parameter structs.

### Prompt Architecture (Core + Hooks)
- `prompts/core.md` — ralph-cli owns this; defines phases, protocol, invariants
- `eng/ralph.md` — each project owns this; domain-specific instructions
- Concatenated at runtime by `internal/prompt/builder.go`
- Gap-fill context appended when retrying a task

### State Machine
Engine state persisted as JSON (`eng/ralph-state.json`):
- `mode`: "normal" or "gap-fill"
- `attempt`: 0–3 (max retries hardcoded at 3)
- `gapDetails`: context injected into prompt on retry
- `lastOutcomes`: rolling window of 20 for stuck detection (2+ consecutive NO_OPs)

### Outcome Classification
`ClassifyOutcome()` in `engine/outcome.go` maps agent results to:
`SUCCESS` | `GAPS_FOUND` | `NO_OP` | `AGENT_CRASH` | `STUCK` | `PRD_COMPLETE`

### TUI
- `tui.Output` interface abstracts TUI vs plain output
- `TUIOutput` uses Bubbletea with a fixed 2-line header + scrolling body
- `PlainOutput` writes to stdout (for `--no-tui`, CI, piped output)
- Styles in `styles.go` use Lipgloss; header color reflects phase

### Config Cascade
`DefaultConfig()` ← `eng/ralph.yaml` ← CLI flags (checked via `cmd.Flags().Changed()`)

## Package Dependencies (internal)

```
cmd/ralph → config, engine, tui
engine    → agent, config, git, tasks, telemetry, prompt, tui
agent     → tasks (types only, via ToolCallbacks)
prompt    → prompts (embed)
```

Packages `config`, `tasks`, `git`, `telemetry`, `tui`, `prompt` have no internal cross-dependencies.

## Build & Test

```bash
make build          # go build -o ralph ./cmd/ralph
make test           # go test ./...
make lint           # golangci-lint run
make install        # go install ./cmd/ralph
make dist           # Cross-compile for darwin/linux × amd64/arm64
```

Version injected via ldflags: `go build -ldflags "-X main.version=1.0.0"`

## When Making Changes

- Run `go build ./...` after any change to verify compilation.
- Run `go test ./...` to verify all tests pass.
- If adding a new package, keep it in `internal/` unless it needs to be importable.
- If adding config fields, update `DefaultConfig()`, `Validate()`, and `ralph.yaml` schema in docs.
- If changing custom tool contracts, update both `agent/tools.go` and `prompts/core.md`.
- If modifying the engine loop, ensure state persistence handles crashes (save state after every iteration).
- The task index markdown format is a contract — changes must be backward-compatible.
