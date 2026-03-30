# Ralph-CLI Design Decisions

## 1. Problem Statement

Ralph is an autonomous loop orchestrator that drives an AI agent through a backlog of implementation tasks. It currently exists as `ralph.mjs` — a Node.js script duplicated across multiple JavaScript projects with ~95% identical code. The only differences between copies are:

- **Task ID formats** (e.g., `T\d+` vs `TASK-\d+` vs custom patterns)
- **Directory paths** (where tasks, logs, and state live)
- **Markdown formatting** (how task specs and indexes are structured)

This duplication creates a maintenance burden: bug fixes, new features, and protocol changes must be manually propagated across every project. When one copy drifts, behavior diverges silently.

**Ralph-CLI extracts this orchestrator into a standalone, reusable Go binary** distributed via npm, so any project can adopt it with `npx ralph` and a small config file — eliminating duplication while preserving the full capability of the original script.

---

## 2. Goals (Ordered by Priority)

1. **Keep tool effectiveness** — The extracted CLI must produce the same quality of autonomous task completion as the per-project `ralph.mjs`. No regression in agent behavior, outcome classification, or retry logic.

2. **Easy to consume in other projects** — A new project should be able to adopt ralph-cli with minimal setup: install via npm, run `ralph init`, edit a small YAML config, and start running tasks. No Go toolchain required.

3. **Fast task completion without compromising quality** — Optimize the loop for throughput (serial execution, efficient state management, smart retries) without sacrificing correctness. A task completed wrong is worse than a task completed slowly.

4. **Polished TUI** — Provide a rich terminal interface that shows real-time progress, streaming agent output, and at-a-glance stats. The operator should always know what Ralph is doing, how far along it is, and whether anything is stuck.

---

## 3. Architecture Decisions

### Decision 1: Language — Go

**Choice:** Go

**Rationale:** Go provides the best combination of properties for this tool:
- **TUI ecosystem** — Bubbletea and Lipgloss are the most mature terminal UI libraries available, with first-class support for streaming, layouts, and styling.
- **Single binary** — `go build` produces a statically linked binary with no runtime dependencies. This is critical for npm distribution — we embed the binary directly, no interpreter needed.
- **Strong concurrency** — Goroutines and channels make it straightforward to manage concurrent concerns (streaming SDK events, updating TUI, writing logs) without callback hell.
- **Fast startup** — Go binaries start in milliseconds, important for a CLI that may be invoked frequently.

Alternatives considered: Node.js (would perpetuate the JS dependency problem), Rust (slower development velocity, TUI ecosystem less mature for this use case), Python (startup time, distribution complexity).

---

### Decision 2: Copilot Integration — Copilot SDK

**Choice:** Copilot SDK (`github.com/github/copilot-sdk/go`)

**Rationale:** The Copilot SDK provides:
- **Structured events** — Instead of parsing raw text streams, the SDK emits typed events (`message_delta`, `tool.execution_partial_result`, etc.) that the TUI can render cleanly.
- **Custom tool registration** — Ralph's outcome protocol is implemented as SDK tools, giving the agent a structured API instead of fragile text conventions.
- **Session management** — The SDK handles conversation context, token limits, and model selection, letting Ralph focus on orchestration logic.
- **Model abstraction** — Switch between models via config without changing orchestration code.

This replaces the raw API calls in `ralph.mjs` with a higher-level abstraction that handles the protocol correctly.

---

### Decision 3: Distribution — npm Wrapper with Embedded Go Binary

**Choice:** npm package that embeds a pre-compiled Go binary, following the esbuild/turbo distribution pattern.

**Rationale:**
- **Target audience** — Ralph's users are developers working in JS/TS projects who already have npm in their workflow. `npx ralph` is the lowest-friction entry point.
- **No Go toolchain required** — Users don't need to install Go. The npm package includes platform-specific binaries (darwin-arm64, darwin-x64, linux-x64, win-x64).
- **Proven pattern** — esbuild, turbo, and other tools successfully distribute Go/Rust binaries via npm. The pattern is well-understood and reliable.
- **Version management** — npm's semver and lockfile system handles versioning automatically.

The `npm/` directory contains the package.json, bin stubs, and platform-specific optional dependencies.

---

### Decision 4: TUI Framework — Bubbletea + Lipgloss

**Choice:** Bubbletea for the application model, Lipgloss for styling.

**Rationale:**
- **Elm architecture** — Bubbletea's Model/Update/View pattern cleanly separates state management from rendering, making the TUI testable and predictable.
- **Streaming support** — Bubbletea handles concurrent message delivery natively, which is essential for rendering streaming SDK events in real-time.
- **Lipgloss styling** — Declarative styling with adaptive color support (true color, 256 color, basic) means the TUI looks good across terminal emulators.
- **Community and maintenance** — Charmbracelet's libraries are actively maintained and widely adopted in the Go CLI ecosystem.

---

### Decision 5: Task Format — Convention with Overrides

**Choice:** Sensible defaults with configurable regex patterns and paths.

**Rationale:**
- **Zero-config start** — Out of the box, Ralph expects tasks in `tasks/` with IDs matching `T\d+` and an index at `tasks/index.md`. This works for most projects without any configuration.
- **Override when needed** — Projects with existing task formats (e.g., `TASK-\d+`, different directory structures) can override via `ralph.yaml` without forking or patching Ralph.
- **Convention over configuration** — The defaults encode the most common pattern observed across existing `ralph.mjs` deployments. Only the ~5% of projects that differ need to touch config.

This directly addresses the root cause of duplication: the original copies differed primarily in these format details.

---

### Decision 6: Outcome Protocol — 5 Custom SDK Tools

**Choice:** Replace fragile XML tag parsing with 5 structured SDK tools.

**Rationale:** In `ralph.mjs`, the agent communicates outcomes by emitting XML-like tags (`<outcome>SUCCESS</outcome>`) in its text output, which Ralph parses with regex. This is fragile:
- The agent sometimes forgets the tags or formats them inconsistently.
- Parsing XML from a text stream is error-prone (partial tags, nested content, encoding issues).
- There's no validation — the agent can emit any string.

SDK tools solve all of these problems:
- The agent **calls a function** instead of emitting text — structured input with typed parameters.
- The SDK validates the call — required parameters are enforced.
- Ralph controls the response — it can confirm, reject, or provide feedback.

The 5 tools form a complete task lifecycle API (see Section 3, Decision 7).

---

### Decision 7: Custom Tools — Task Lifecycle API

**Choice:** Register 5 custom tools with the Copilot SDK:

```
ralph_list_tasks()                              → [{id, title, status, deps, blocked_reason?}]
ralph_pick_task()                               → {id, title, spec_content, attempt, gap_context?}
ralph_get_task_spec(task_id)                     → {id, title, full_markdown_content}
ralph_update_status(task_id, status, reason?)    → confirmation
ralph_report_outcome(task_id, outcome, summary)  → confirmation
```

**Rationale:**
- **`ralph_list_tasks()`** — Returns the current state of all tasks. The agent uses this to understand what's available, what's blocked, and what's done. Ralph generates this from its internal state, ensuring consistency.
- **`ralph_pick_task()`** — Ralph selects the next task based on priority, dependencies, and retry state. The agent doesn't choose — Ralph does. This prevents the agent from cherry-picking easy tasks or getting stuck in loops.
- **`ralph_get_task_spec(task_id)`** — Returns the full specification for a task. Separating this from `pick_task` allows the agent to re-read specs during implementation without re-picking.
- **`ralph_update_status(task_id, status, reason?)`** — The agent reports status transitions (in-progress, blocked, etc.) as they happen, not just at the end. This enables real-time TUI updates.
- **`ralph_report_outcome(task_id, outcome, summary)`** — The agent reports the final outcome of a task attempt. The outcome is one of the classified types (SUCCESS, GAPS_FOUND, etc.). Ralph uses this to drive retry logic and state transitions.

---

### Decision 8: Task Index Ownership — Ralph Owns It

**Choice:** Ralph owns the task index (`tasks/index.md`) via custom tools. The agent never directly parses or edits the index file.

**Rationale:**
- **Single source of truth** — Ralph's internal state is authoritative. The index file is a serialized view of that state, not the other way around.
- **No parse errors** — The agent interacting with tasks through tools means it never needs to parse markdown tables, match regex patterns, or handle edge cases in index formatting.
- **Atomic updates** — Ralph updates the index file atomically after each state change, preventing partial writes or corruption.
- **Consistency** — The agent sees tasks through a clean API. Ralph can change the index format without affecting agent behavior.

---

### Decision 9: Configuration — `eng/ralph.yaml`

**Choice:** YAML configuration file at `eng/ralph.yaml`.

**Rationale:**
- **YAML over JSON** — YAML supports comments, which are valuable for documenting config choices in-repo. JSON does not.
- **YAML over TOML** — YAML is more widely known and has better Go library support. The config structure is simple enough that TOML's advantages (strict typing) don't apply.
- **`eng/` directory** — Keeps Ralph's configuration alongside other engineering tooling, separate from application source code. This is consistent with where `ralph.mjs` deployments typically store their config.
- **Single file** — One config file is simpler than a directory of configs. The schema is small enough (see Section 5) that splitting would add complexity without benefit.

---

### Decision 10: Prompt Architecture — Core + Hooks via Concatenation

**Choice:** Ralph-cli owns the core protocol prompt. Projects append domain-specific instructions via a hook file. The final prompt is a simple concatenation: `core.md + project.md`.

**Rationale:**
- **Protocol stability** — The core prompt defines Ralph's behavior: how to use tools, how to report outcomes, how to handle errors. This must be consistent across all projects. Ralph ships this prompt and the project cannot override it.
- **Domain flexibility** — Projects need to inject domain-specific context: "this is a React app", "use these coding conventions", "avoid these patterns". The hook file (`eng/ralph.md` by default) provides this.
- **Simple composition** — Concatenation is the simplest possible composition strategy. No template engine, no variable substitution, no inheritance. The core prompt ends with a section header, the project prompt continues from there.
- **Versioned independently** — The core prompt ships with Ralph and upgrades automatically. The project prompt is version-controlled in the project repo.

---

### Decision 11: Model Selection — Config Default + CLI Override

**Choice:** Default model specified in `ralph.yaml`, overridable with `--model` CLI flag.

**Rationale:**
- **Config for consistency** — The YAML default ensures all team members use the same model unless they have a reason to override.
- **CLI for experimentation** — `--model` lets operators test different models without modifying checked-in config. Useful for comparing model performance on specific task types.
- **Empty default** — When `model` is empty in config, Ralph uses the Copilot SDK's default model. This keeps the config minimal for projects that don't need to pin a model.

---

### Decision 12: Gap Review — Opt-In

**Choice:** Gap review is disabled by default, enabled with `--review` flag or `review.enabled: true` in config.

**Rationale:**
- **Speed by default** — Most task iterations don't need a separate review pass. Enabling it by default would add latency to every iteration.
- **Quality when needed** — For critical tasks or final passes, gap review provides a second-look mechanism where Ralph re-evaluates the agent's work against the task spec.
- **Opt-in via flag or config** — `--review` for one-off use, `review.enabled` for projects that always want it. The review prompt can be customized via `review.prompt`.

---

### Decision 13: Permissions — Auto-Approve All

**Choice:** All agent actions are auto-approved. No confirmation prompts.

**Rationale:**
- **Autonomous loop** — Ralph is designed to run unattended. Confirmation prompts would block the loop and defeat the purpose.
- **Trust boundary** — The agent operates within the project directory with the permissions of the invoking user. The trust model is: if you run Ralph, you trust it to modify your project.
- **Git as safety net** — All changes are committed to git. If Ralph makes a mistake, `git revert` or `git reset` recovers instantly. The git integration (auto-stash, batch push) provides the undo mechanism.

---

### Decision 14: Parallelism — Serial Only

**Choice:** Tasks are executed serially, one at a time.

**Rationale:**
- **Dependency correctness** — Tasks often depend on each other (task B reads files created by task A). Parallel execution would require sophisticated dependency resolution and conflict detection.
- **Agent context** — The Copilot SDK manages a single conversation context. Parallel tasks would require multiple sessions with potential conflicts in the working directory.
- **Simplicity** — Serial execution is dramatically simpler to implement, debug, and reason about. The loop is already fast enough — the bottleneck is agent thinking time, not orchestrator overhead.
- **Future option** — The architecture doesn't preclude future parallelism. The task tools and state management are designed to be session-scoped, so multiple loops could run with coordination.

---

### Decision 15: State Persistence — JSON File

**Choice:** State is persisted to `eng/ralph-state.json`.

**Rationale:**
- **Human-readable** — JSON is easy to inspect and debug when something goes wrong. Operators can `cat` the state file to understand what Ralph thinks is happening.
- **Atomic writes** — Write to a temp file, then rename. This prevents corruption from crashes or power loss.
- **Git-friendly** — The state file can be committed to track Ralph's progress across sessions, or `.gitignore`d for ephemeral runs.
- **No external dependencies** — No database, no Redis, no SQLite. A single file is the simplest possible persistence mechanism for the small amount of state Ralph manages.

---

### Decision 16: Git Management — Full, Configurable

**Choice:** Ralph manages git operations end-to-end: commits after each task, batch pushes, auto-stash of uncommitted changes, dirty-tree detection. All configurable.

**Rationale:**
- **Commit per task** — Each completed task gets its own commit with a structured message. This provides clean history and easy revert granularity.
- **Batch push** — Pushing after every commit is slow and noisy. `push_every: 5` (configurable) batches pushes for efficiency while keeping the remote reasonably up-to-date.
- **Auto-stash** — When `auto_stash: true`, Ralph stashes uncommitted changes before starting and restores them after. This prevents conflicts between manual work and Ralph's changes.
- **Skip push** — `skip_push: true` disables pushing entirely, useful for local-only runs or CI environments that handle pushing separately.
- **All optional** — Every git feature can be disabled. Projects with unusual git workflows can turn off what they don't need.

---

### Decision 17: Telemetry — Rich JSONL + TUI Dashboard

**Choice:** JSONL telemetry logs with a TUI dashboard showing 5-run rolling baseline deltas.

**Rationale:**
- **JSONL format** — One JSON object per line, append-only. Easy to parse with `jq`, easy to ingest into observability tools, no corruption risk from concurrent writes.
- **Per-task metrics** — Each task attempt logs: duration, token usage, outcome, retry count, model used. This enables analysis of which task types are expensive or failure-prone.
- **5-run rolling baseline** — The TUI shows how the current run compares to the rolling average of the last 5 runs. This surfaces regressions (e.g., "tasks are taking 2x longer than usual") without requiring external dashboards.
- **`eng/logs/` directory** — Telemetry lives alongside other Ralph artifacts. `retention_days: 7` auto-cleans old logs.

---

### Decision 18: Logging — `eng/logs/` Directory

**Choice:** All logs written to `eng/logs/` with configurable retention.

**Rationale:**
- **Structured directory** — Logs are organized by run timestamp. Each run gets a directory with the JSONL telemetry file and per-task agent conversation logs.
- **Retention policy** — `retention_days: 7` automatically deletes logs older than 7 days. This prevents unbounded disk growth without requiring manual cleanup.
- **Debug-friendly** — When a task fails, the full agent conversation is available in the logs for post-mortem analysis. This is critical for improving prompts and debugging agent behavior.
- **Separate from application logs** — Ralph's logs don't pollute the project's log directories.

---

### Decision 19: CLI Subcommands — Minimal Surface

**Choice:** Three subcommands: `ralph` (defaults to `ralph run`), `ralph init`, `ralph status`.

**Rationale:**
- **`ralph` / `ralph run`** — The primary command. Starts the autonomous loop. Bare `ralph` defaults to `ralph run` because it's the most common operation.
- **`ralph init`** — Scaffolds a new project: creates `eng/ralph.yaml`, `eng/ralph.md` (prompt template), `tasks/` directory, and `tasks/index.md` template. Lowers the barrier to adoption.
- **`ralph status`** — Prints current state: how many tasks are done, remaining, blocked, last run stats. Useful for CI integration and quick checks without starting a full run.
- **No more** — Every additional subcommand increases cognitive load. These three cover the complete lifecycle: setup → run → check. Additional operations (e.g., "retry a specific task") are better handled through config or flags on `run`.

---

### Decision 20: Scaffolding — Full Project Bootstrap

**Choice:** `ralph init` scaffolds a complete Ralph setup: config file, prompt template, tasks directory, and index template.

**Rationale:**
- **Zero-to-running in 60 seconds** — A developer should be able to `npx ralph init`, write a task spec, and `npx ralph` within a minute.
- **Correct defaults** — The scaffolded config uses sensible defaults (see Section 5). The prompt template includes instructional comments showing what to customize.
- **Index template** — The scaffolded `tasks/index.md` includes the expected format with example entries, so the developer sees the pattern immediately.
- **Idempotent** — Running `ralph init` in a project that already has Ralph config is safe — it skips existing files and reports what was skipped.

---

## 4. TUI Design

### Layout: Split-Pane

The TUI uses a fixed 2-line stats header with a scrolling agent output body below.

**Header (fixed, 2 lines):**
- Line 1: Iteration counter, current task name, task timer, total run timer.
- Line 2: Done/remaining/retry/blocked counts, active model name.

**Body (scrolling):**
- Renders SDK structured events in real-time.
- `message_delta` events render as streaming text.
- `tool.execution_partial_result` events render with tool name header and indented output.
- Color-coded by event type (tool calls in cyan, errors in red, success in green).

### Header Mockup

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

### Header Fields

| Field | Source | Update Frequency |
|-------|--------|-----------------|
| Iteration (`#3/50`) | Loop counter / `iterations` config | Per iteration |
| Task name (`T005: Add auth middleware`) | `ralph_pick_task()` result | Per task |
| Task timer (`⏱ 2m 14s`) | Wall clock since task start | Every second |
| Total timer (`Total: 18m 32s`) | Wall clock since run start | Every second |
| Done count (`✓ 4 done`) | Internal state | On status change |
| Remaining count (`12 remaining`) | Internal state | On status change |
| Retry count (`↻ 1 retry`) | Internal state | On status change |
| Blocked count (`■ 0 blocked`) | Internal state | On status change |
| Model (`claude-sonnet-4`) | Config / `--model` flag | Static per run |

---

## 5. Configuration Schema — `ralph.yaml`

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

1. **Built-in defaults** — Hardcoded in Go, matching the schema above.
2. **`eng/ralph.yaml`** — Project-level config, checked into git.
3. **CLI flags** — `--model`, `--iterations`, `--review`, etc. override config values.

---

## 6. Project Structure

```
ralph-cli/
├── cmd/ralph/main.go          # Entry point, CLI flag parsing, subcommand routing
├── internal/
│   ├── config/                # ralph.yaml loading, defaults, validation
│   │   ├── config.go          #   Config struct + Load() function
│   │   └── defaults.go        #   Default values
│   ├── engine/                # Main loop orchestrator
│   │   ├── engine.go          #   Engine struct, Run() method
│   │   ├── outcome.go         #   Outcome classification + routing
│   │   └── loop.go            #   Single iteration logic
│   ├── agent/                 # Copilot SDK session + custom tools
│   │   ├── session.go         #   SDK session creation + management
│   │   ├── tools.go           #   Custom tool registration
│   │   └── events.go          #   Event stream handling
│   ├── tasks/                 # Task index parsing, status management
│   │   ├── index.go           #   Index file parsing + writing
│   │   ├── task.go            #   Task struct + status types
│   │   └── picker.go          #   Task selection logic (deps, priority)
│   ├── git/                   # Git operations
│   │   ├── git.go             #   Commit, push, stash operations
│   │   └── batch.go           #   Batch push logic
│   ├── tui/                   # Bubbletea model + views
│   │   ├── model.go           #   Bubbletea Model implementation
│   │   ├── header.go          #   Stats header rendering
│   │   ├── body.go            #   Agent output rendering
│   │   └── styles.go          #   Lipgloss style definitions
│   ├── telemetry/             # JSONL logging + baseline comparison
│   │   ├── logger.go          #   JSONL event writer
│   │   ├── metrics.go         #   Per-task metrics collection
│   │   └── baseline.go        #   5-run rolling baseline calculation
│   └── prompt/                # Core prompt + project prompt loading
│       ├── loader.go          #   Prompt file loading + concatenation
│       └── core.go            #   Embedded core prompt
├── prompts/
│   └── core.md                # Core protocol prompt (shipped with binary)
├── npm/
│   ├── package.json           #   npm package definition
│   ├── bin/ralph              #   Bin stub (resolves platform binary)
│   └── platforms/             #   Platform-specific optional deps
├── go.mod
├── go.sum
├── Makefile                   # Build targets: build, test, lint, release
└── README.md
```

### Package Responsibilities

| Package | Responsibility | Key Types |
|---------|---------------|-----------|
| `config` | Load, validate, and merge configuration from defaults, YAML, and CLI flags | `Config`, `TasksConfig`, `GitConfig` |
| `engine` | Orchestrate the main loop: iterate, route outcomes, manage state | `Engine`, `Outcome`, `RunState` |
| `agent` | Create Copilot SDK sessions, register custom tools, handle events | `Session`, `ToolHandler` |
| `tasks` | Parse task index, manage task status, select next task | `Task`, `Index`, `Picker` |
| `git` | Execute git operations: commit, push, stash, dirty detection | `GitOps`, `BatchPusher` |
| `tui` | Render the terminal UI: header stats, streaming output, layout | `Model`, `HeaderView`, `BodyView` |
| `telemetry` | Write JSONL logs, collect metrics, compute baseline deltas | `Logger`, `TaskMetrics`, `Baseline` |
| `prompt` | Load and concatenate core + project prompts | `Loader` |

---

## 7. Main Loop Flow

```
┌──────────────┐
│  Load Config │  eng/ralph.yaml + CLI flags + defaults
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Load Prompts │  prompts/core.md + eng/ralph.md → concatenated prompt
└──────┬───────┘
       │
       ▼
┌──────────────────┐
│ Create SDK Session│  Initialize Copilot SDK with prompt + model
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ Register Tools   │  Register 5 custom tools with SDK session
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ Load/Init State  │  Read eng/ralph-state.json or create fresh
└──────┬───────────┘
       │
       ▼
┌──────────────────────────────────────────────────┐
│                 ITERATION LOOP                    │
│  for i := 0; i < config.Iterations; i++ {        │
│                                                   │
│  ┌────────────────┐                               │
│  │  Pick Task     │  ralph_pick_task() via engine │
│  └──────┬─────────┘                               │
│         │                                         │
│         ▼                                         │
│  ┌────────────────┐                               │
│  │ Stream Events  │  SDK runs agent, streams      │
│  │                │  message_delta + tool calls    │
│  │                │  TUI renders in real-time      │
│  └──────┬─────────┘                               │
│         │                                         │
│         ▼                                         │
│  ┌─────────────────┐                              │
│  │ Parse Outcome   │  ralph_report_outcome() call │
│  │                 │  from agent via custom tool   │
│  └──────┬──────────┘                              │
│         │                                         │
│         ▼                                         │
│  ┌─────────────────┐                              │
│  │ Classify        │  Map to outcome type:        │
│  │                 │  SUCCESS / GAPS_FOUND /       │
│  │                 │  NO_OP / AGENT_CRASH /        │
│  │                 │  STUCK / PRD_COMPLETE         │
│  └──────┬──────────┘                              │
│         │                                         │
│         ▼                                         │
│  ┌─────────────────┐                              │
│  │ Route Outcome   │  See routing table below     │
│  └──────┬──────────┘                              │
│         │                                         │
│         ▼                                         │
│  ┌─────────────────┐                              │
│  │ Persist State   │  Write eng/ralph-state.json  │
│  └──────┬──────────┘                              │
│         │                                         │
│         ▼                                         │
│  ┌─────────────────┐                              │
│  │ Git Operations  │  Commit + conditional push   │
│  └──────┬──────────┘                              │
│         │                                         │
│         ▼                                         │
│  ┌─────────────────┐                              │
│  │ Log Telemetry   │  Append to JSONL log         │
│  └──────┘──────────┘                              │
│  }                                                │
└──────────────────────────────────────────────────┘
```

### Outcome Routing Table

| Outcome | Action | Next State |
|---------|--------|------------|
| `SUCCESS` | Mark task done, update index, move to next task | Normal mode, next task |
| `GAPS_FOUND` | Increment attempt, inject gap context, retry same task | Gap-fill mode, same task |
| `NO_OP` | Increment attempt, check for stuck condition | See stuck detection |
| `AGENT_CRASH` | Log error, increment attempt, retry with clean context | Normal mode, same task |
| `STUCK` | Mark task blocked with reason, move to next task | Normal mode, next task |
| `PRD_COMPLETE` | All tasks done, exit loop | Run complete |

---

## 8. State Machine

### State Shape

```json
{
  "mode": "normal",
  "currentTaskId": "T005",
  "attempt": 1,
  "maxAttempts": 3,
  "gapDetails": null,
  "blockedTasks": [
    {
      "taskId": "T003",
      "reason": "Missing API endpoint specification",
      "blockedAt": "2024-01-15T10:30:00Z"
    }
  ],
  "lastOutcomes": [
    { "taskId": "T004", "outcome": "SUCCESS", "timestamp": "..." },
    { "taskId": "T005", "outcome": "GAPS_FOUND", "timestamp": "..." }
  ],
  "completedTasks": ["T001", "T002", "T004"],
  "totalIterations": 12,
  "runStartedAt": "2024-01-15T10:00:00Z"
}
```

### State Fields

| Field | Type | Description |
|-------|------|-------------|
| `mode` | `"normal" \| "gap-fill"` | Current operating mode. Normal = working on a fresh task. Gap-fill = retrying with gap context. |
| `currentTaskId` | `string` | ID of the task currently being worked on. |
| `attempt` | `int` | Current attempt number for the current task (1-indexed). |
| `maxAttempts` | `int` | Maximum attempts per task before marking blocked. Default: 3. |
| `gapDetails` | `object \| null` | When in gap-fill mode, contains the gaps identified in the previous attempt. Injected into the agent's context for the retry. |
| `blockedTasks` | `array` | Tasks that have been marked blocked, with reasons and timestamps. |
| `lastOutcomes` | `array` | Rolling window of the last 20 outcomes. Used for stuck detection and telemetry baseline calculations. |
| `completedTasks` | `array` | IDs of all successfully completed tasks. |
| `totalIterations` | `int` | Total iterations executed in this run. |
| `runStartedAt` | `string` | ISO 8601 timestamp of when the current run started. |

### Mode Transitions

```
                    ┌───────────────┐
         ┌─────────│    NORMAL     │◄──────────┐
         │         └──────┬────────┘           │
         │                │                     │
         │         pick_task()                  │
         │                │                     │
         │                ▼                     │
         │         ┌──────────────┐            │
         │         │  EXECUTING   │            │
         │         └──────┬───────┘            │
         │                │                     │
         │         report_outcome()             │
         │                │                     │
         │     ┌──────────┼──────────┐         │
         │     │          │          │         │
         │     ▼          ▼          ▼         │
         │  SUCCESS   GAPS_FOUND   NO_OP      │
         │     │          │          │         │
         │     │          │     attempt < 3?   │
         │     │          │     ┌─yes──┘       │
         │     │          │     │    no──► STUCK ──► blocked
         │     │          ▼     ▼              │
         │     │   ┌──────────────┐            │
         │     │   │  GAP-FILL    │            │
         │     │   │  (retry w/   │            │
         │     │   │   context)   │            │
         │     │   └──────┬───────┘            │
         │     │          │                     │
         │     │   report_outcome()             │
         │     │          │                     │
         │     └──────────┴─────────────────────┘
         │
         │  (all tasks done or iterations exhausted)
         ▼
   ┌──────────────┐
   │  COMPLETED   │
   └──────────────┘
```

---

## 9. Retry Logic

### Rules

1. **Maximum 3 attempts per task** — Each task gets at most `maxAttempts` (default: 3) attempts before being marked as blocked. This prevents infinite loops on tasks the agent cannot complete.

2. **Gap-fill mode injects context** — When the outcome is `GAPS_FOUND`, Ralph switches to gap-fill mode. The gap details from `ralph_report_outcome()` are stored in state and injected into the agent's context for the next attempt via `ralph_pick_task()`. The agent sees what it tried, what failed, and what gaps remain.

3. **Stuck detection: 2+ consecutive NO_OPs → blocked** — If the agent produces 2 or more consecutive `NO_OP` outcomes (no meaningful progress), Ralph marks the task as blocked with reason "stuck — no progress after N attempts". This catches cases where the agent is confused but not crashing.

### Retry Flow

```
Attempt 1 (normal mode):
  Agent works on task T005
  Outcome: GAPS_FOUND ("tests fail — missing mock for auth service")

Attempt 2 (gap-fill mode):
  Agent receives: task spec + gap context from attempt 1
  "Previous attempt found gaps: tests fail — missing mock for auth service"
  Outcome: GAPS_FOUND ("auth mock added but integration test still fails")

Attempt 3 (gap-fill mode):
  Agent receives: task spec + gap context from attempts 1 & 2
  "Previous attempts found gaps:
   1. tests fail — missing mock for auth service
   2. auth mock added but integration test still fails"
  Outcome: SUCCESS ✓ → task marked done, move to next
```

### Failure Scenarios

| Scenario | Detection | Action |
|----------|-----------|--------|
| Agent finds gaps | `GAPS_FOUND` outcome | Retry with gap context (up to maxAttempts) |
| Agent makes no progress | 2+ consecutive `NO_OP` | Mark blocked, move to next task |
| Agent crashes/errors | `AGENT_CRASH` outcome or SDK error | Retry with clean context (up to maxAttempts) |
| All retries exhausted | `attempt >= maxAttempts` | Mark blocked with accumulated gap details |
| Agent reports all done | `PRD_COMPLETE` outcome | Exit loop, report final stats |

### Gap Context Accumulation

Gap context is cumulative — each retry sees all previous gap details, not just the most recent. This gives the agent full history of what has been tried and what failed, enabling it to try different approaches rather than repeating the same mistake.

```go
type GapDetails struct {
    Attempt   int       `json:"attempt"`
    Gaps      string    `json:"gaps"`
    Timestamp time.Time `json:"timestamp"`
}

// State accumulates gap details across attempts
type TaskRetryState struct {
    TaskID     string       `json:"task_id"`
    Attempt    int          `json:"attempt"`
    GapHistory []GapDetails `json:"gap_history"`
}
```
