# ralph-cli

Autonomous task completion loop powered by GitHub Copilot SDK.

---

## What is Ralph?

Ralph is a CLI tool that runs GitHub Copilot in an autonomous loop to complete tasks from a backlog. Each iteration, the agent picks a task, implements it, validates it, and reports the outcome. Ralph manages retries, gap-fill, git operations, and telemetry — you just write task specs and let it run.

**Key features:**

- 🔄 **Autonomous loop** — runs unattended, completing tasks one by one serially
- 🔧 **Custom SDK tools** — structured communication via 5 typed tools (no XML parsing)
- 🖥️ **Polished TUI** — real-time stats header, live agent output streaming via Bubbletea
- 🔁 **Gap-fill retries** — automatically retries tasks with context about what failed
- 📊 **Telemetry** — JSONL logging with 5-run rolling baseline comparison
- ⚡ **Convention over configuration** — works out of the box, customizable when needed

---

## Quick Start

```bash
# Install via npm (no Go toolchain required)
npm install ralph-cli

# Or install via Go
go install github.com/mane/ralph-cli/cmd/ralph@latest

# Initialize a project
cd your-project
ralph init

# Edit your project prompt
vim eng/ralph.md

# Add tasks to tasks/index.md and create spec files in tasks/
# Then run:
ralph
```

---

## Installation

### npm (recommended)

```bash
npm install ralph-cli
```

The npm package includes pre-compiled platform-specific binaries (darwin-arm64, darwin-x64, linux-x64, linux-arm64). No Go toolchain required.

### Go

```bash
go install github.com/mane/ralph-cli/cmd/ralph@latest
```

### From source

```bash
git clone https://github.com/mane/ralph-cli.git
cd ralph-cli
make install
```

---

## Configuration

Ralph is configured via `eng/ralph.yaml`. The file is located by searching from the current directory upward (like `.git`). If the file doesn't exist, built-in defaults are used.

```yaml
# Model to use for the agent. Empty string uses SDK default.
model: ""

# Maximum number of iterations (task attempts) per run.
iterations: 50

# Path to the project-specific prompt file (appended to core prompt).
prompt: "eng/ralph.md"

# Task discovery and formatting.
tasks:
  # Directory containing task spec files.
  directory: "tasks"
  # Path to the task index file.
  index: "tasks/index.md"
  # Regex pattern for matching task IDs in the index.
  id_pattern: "T\\d+"

# Git integration settings.
git:
  # Push to remote every N completed tasks. 0 = push after every task.
  push_every: 5
  # Stash uncommitted changes before starting, restore after.
  auto_stash: false
  # Disable pushing entirely (commits still happen locally).
  skip_push: false

# Gap review settings.
review:
  # Enable gap review pass after task completion.
  enabled: false
  # Custom review prompt (empty = use default review prompt).
  prompt: ""

# Logging and telemetry.
logs:
  # Directory for run logs and telemetry.
  directory: "eng/logs"
  # Auto-delete logs older than this many days. 0 = keep forever.
  retention_days: 7
```

### Config Resolution Order

1. **Built-in defaults** — hardcoded, matching the schema above.
2. **`eng/ralph.yaml`** — project-level config, checked into git.
3. **CLI flags** — `--model`, `--iterations`, `--review`, etc. override config values.

---

## Task Format

### Task Index

Ralph manages tasks through a markdown table at `tasks/index.md` (configurable). The agent interacts with tasks exclusively via custom tools — it never edits the index directly.

Example `tasks/index.md`:

```markdown
| ID   | Title                        | Status      | Deps |
|------|------------------------------|-------------|------|
| T001 | Set up project structure     | done        |      |
| T002 | Implement config loader      | done        | T001 |
| T003 | Add authentication middleware| pending     | T001 |
| T004 | Create REST API endpoints    | pending     | T003 |
| T005 | Write integration tests      | pending     | T004 |
```

### Task Spec Files

Each task has a corresponding spec file in the tasks directory (e.g., `tasks/T003.md`). Specs contain the full requirements, acceptance criteria, and any relevant context:

```markdown
# T003: Add authentication middleware

## Description
Implement JWT-based authentication middleware for the API server.

## Acceptance Criteria
- Middleware validates JWT tokens on protected routes
- Invalid tokens return 401 with structured error response
- Token expiry is handled gracefully
- Unit tests cover valid, expired, and malformed tokens
```

Task IDs are matched by the `id_pattern` regex (default: `T\d+`). Projects with different ID formats can override this in config.

---

## Prompt Architecture

Ralph uses a two-layer prompt system:

1. **Core prompt** (`prompts/core.md`) — shipped with the binary. Defines the agent's identity, the 7-phase protocol (Orient → Study → Design → Implement → Validate → Record → Exit), invariants, gap-fill mode behavior, and quality standards. This is not editable by projects.

2. **Project prompt** (`eng/ralph.md`) — provided by each project. Contains domain-specific instructions: tech stack, coding conventions, testing patterns, architectural constraints.

At runtime, Ralph concatenates: `core.md + project.md` → final system prompt. The core prompt ends with a section header and the project prompt continues from there.

---

## Custom Tools

The agent communicates with Ralph through 5 structured SDK tools. These replace the fragile XML-tag parsing used in the original `ralph.mjs`:

| Tool | Signature | Purpose |
|------|-----------|---------|
| `ralph_list_tasks` | `() → [{id, title, status, deps, blocked_reason?}]` | List all tasks with their current status |
| `ralph_pick_task` | `() → {id, title, spec_content, attempt, gap_context?}` | Get the next eligible task (respects deps, skips blocked) |
| `ralph_get_task_spec` | `(task_id) → {id, title, full_markdown_content}` | Read a specific task spec |
| `ralph_update_status` | `(task_id, status, reason?) → confirmation` | Update task status to `in-progress`, `done`, or `blocked` |
| `ralph_report_outcome` | `(task_id, outcome, summary) → confirmation` | Report iteration outcome: `success`, `stuck`, or `blocked` |

The agent calls `ralph_pick_task` at the start of each iteration and `ralph_report_outcome` at the end. Ralph controls task selection — the agent doesn't choose which task to work on.

---

## CLI Reference

```
ralph              # Run the autonomous loop (default)
ralph run          # Same as above, explicit subcommand
ralph init         # Scaffold project files (eng/ralph.yaml, eng/ralph.md, tasks/)
ralph status       # Show current state (done/remaining/blocked counts, last run stats)
```

### Flags (apply to `ralph` and `ralph run`)

| Flag | Default | Description |
|------|---------|-------------|
| `--iterations` | `50` | Maximum number of iterations per run |
| `--model` | `""` | Model to use for completions (overrides config) |
| `--review` | `false` | Enable gap review mode |
| `--push-every` | `5` | Push to remote every N completed tasks |
| `--auto-stash` | `false` | Stash uncommitted changes before starting |
| `--skip-push` | `false` | Disable pushing entirely |
| `--no-tui` | `false` | Disable TUI, use plain text output |

---

## TUI

Ralph features a split-pane terminal interface built with [Bubbletea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss).

### Layout

**Header (fixed, 2 lines):** iteration counter, current task, timers, and aggregate stats.

**Body (scrolling):** real-time streaming of SDK events — agent text, tool calls, and command output, color-coded by type.

```
┌─────────────────────────────────────────────────────────────────────────┐
│ ▶ Ralph #3/50 │ T005: Add auth middleware │ ⏱ 2m 14s │ Total: 18m 32s │
│ ✓ 4 done │ 12 remaining │ ↻ 1 retry │ ■ 0 blocked │ claude-sonnet-4 │
└─────────────────────────────────────────────────────────────────────────┘
🔧 bash: npm run test
────────────────────
  PASS src/auth.test.ts (2.3s)
  ...streaming output...
```

Disable the TUI with `--no-tui` for CI environments or piped output.

---

## How It Works

```
Load Config (eng/ralph.yaml + CLI flags + defaults)
       │
       ▼
Load Prompts (prompts/core.md + eng/ralph.md → concatenated prompt)
       │
       ▼
Create SDK Session (initialize Copilot SDK with prompt + model)
       │
       ▼
Register 5 Custom Tools
       │
       ▼
Load/Init State (eng/ralph-state.json)
       │
       ▼
┌─── ITERATION LOOP ──────────────────────────────┐
│  1. Pick Task    — ralph_pick_task() via engine  │
│  2. Stream       — SDK runs agent, streams       │
│                    events to TUI in real-time     │
│  3. Outcome      — agent calls ralph_report_     │
│                    outcome() via custom tool      │
│  4. Classify     — SUCCESS / GAPS_FOUND / NO_OP  │
│                    / AGENT_CRASH / STUCK /        │
│                    PRD_COMPLETE                   │
│  5. Route        — retry / next / block          │
│  6. Persist      — write eng/ralph-state.json    │
│  7. Git          — commit + conditional push     │
│  8. Telemetry    — append to JSONL log           │
└──────────────────────────────────────────────────┘
```

### Outcome Routing

| Outcome | Action |
|---------|--------|
| `SUCCESS` | Mark task done, move to next task |
| `GAPS_FOUND` | Inject gap context, retry same task |
| `NO_OP` | Increment attempt, check stuck condition |
| `AGENT_CRASH` | Log error, retry with clean context |
| `STUCK` | Mark task blocked, move to next task |
| `PRD_COMPLETE` | All tasks done, exit loop |

Tasks get up to 3 attempts (configurable) before being marked blocked.

---

## Project Structure

```
ralph-cli/
├── cmd/ralph/main.go          # CLI entrypoint + subcommand routing
├── internal/
│   ├── config/                # ralph.yaml loading, defaults, validation
│   ├── engine/                # Main loop orchestrator + outcome routing
│   ├── agent/                 # Copilot SDK session + custom tool registration
│   ├── tasks/                 # Task index parsing, status management, picker
│   ├── git/                   # Commit, push, stash, dirty detection
│   ├── tui/                   # Bubbletea model, header, body, styles
│   ├── telemetry/             # JSONL logging, metrics, baseline comparison
│   ├── state/                 # State persistence (eng/ralph-state.json)
│   └── prompt/                # Core + project prompt loading/concatenation
├── prompts/core.md            # Built-in agent protocol prompt
├── npm/                       # npm wrapper with platform-specific binaries
├── docs/                      # Design documentation
├── Makefile                   # build, test, lint, install, dist targets
└── README.md
```

---

## License

MIT
