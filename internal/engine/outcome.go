package engine

import (
	"github.com/mane/ralph-cli/internal/agent"
)

const (
	OutcomeSuccess    = "SUCCESS"
	OutcomeGapsFound  = "GAPS_FOUND"
	OutcomeNoOp       = "NO_OP"
	OutcomeAgentCrash = "AGENT_CRASH"
	OutcomeStuck      = "STUCK"
	OutcomePRDComplete = "PRD_COMPLETE"
)

// ClassifyOutcome maps an agent run result into one of the engine-level
// outcome constants.
func ClassifyOutcome(result *agent.RunResult, commitCount int, state *State, err error) string {
	if err != nil {
		return OutcomeAgentCrash
	}
	if result == nil {
		return OutcomeAgentCrash
	}

	switch result.Outcome {
	case "success":
		if result.TaskID != "" {
			return OutcomeSuccess
		}
	case "stuck":
		if state.IsStuck() {
			return OutcomeStuck
		}
		return OutcomeNoOp
	case "blocked":
		return OutcomeStuck
	}

	if commitCount == 0 {
		return OutcomeNoOp
	}
	return OutcomeSuccess
}
