package prompt

import (
	"fmt"
	"os"
	"strings"

	"github.com/mane/ralph-cli/prompts"
)

// GapContext holds context for gap-fill mode when retrying a previously
// attempted task.
type GapContext struct {
	TaskID      string
	Attempt     int
	MaxAttempts int
	GapDetails  string
}

// Build constructs the full prompt for an iteration by concatenating:
//  1. The embedded core prompt
//  2. The project-specific prompt (if projectPromptPath is non-empty)
//  3. A gap-fill section (if gapCtx is non-nil)
func Build(projectPromptPath string, gapCtx *GapContext) (string, error) {
	var b strings.Builder

	b.WriteString(prompts.CorePrompt)

	if projectPromptPath != "" {
		data, err := os.ReadFile(projectPromptPath)
		if err != nil {
			return "", fmt.Errorf("reading project prompt: %w", err)
		}
		b.WriteString("\n\n---\n\n")
		b.WriteString(string(data))
	}

	if gapCtx != nil {
		b.WriteString("\n\n---\n\n")
		fmt.Fprintf(&b, `## GAP-FILL MODE

You are retrying task **%s** (attempt %d/%d).

The previous implementation had these gaps:

%s

**Fix the gaps listed above.** Do NOT start a new task. Do NOT restart the implementation from scratch.
Focus on the specific issues identified.
`, gapCtx.TaskID, gapCtx.Attempt, gapCtx.MaxAttempts, gapCtx.GapDetails)
	}

	return b.String(), nil
}
