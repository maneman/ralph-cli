# ralph-cli

Autonomous task completion loop powered by GitHub Copilot.

## Installation

```bash
npm install ralph-cli
```

Or install the Go binary directly:

```bash
go install github.com/mane/ralph-cli/cmd/ralph@latest
```

## Usage

```bash
# Initialize a new project
ralph init

# Start the autonomous loop
ralph

# With options
ralph --iterations 20 --model claude-sonnet-4
```

See the [full documentation](https://github.com/mane/ralph-cli) for details.
