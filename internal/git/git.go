package git

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Git wraps git CLI operations scoped to a working directory.
type Git struct {
	dir string
}

// New returns a Git instance rooted at dir.
func New(dir string) *Git {
	return &Git{dir: dir}
}

func (g *Git) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", &GitError{Args: args, Output: string(out), Err: err}
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *Git) runSilent(args ...string) error {
	_, err := g.run(args...)
	return err
}

// GitError captures a failed git command for diagnostics.
type GitError struct {
	Args   []string
	Output string
	Err    error
}

func (e *GitError) Error() string {
	return "git " + strings.Join(e.Args, " ") + ": " + e.Err.Error() + "\n" + e.Output
}

func (e *GitError) Unwrap() error { return e.Err }

// IsDirty returns true when the working tree has uncommitted changes.
func (g *Git) IsDirty() (bool, error) {
	out, err := g.run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// Stash saves uncommitted changes with a ralph-specific message.
func (g *Git) Stash() error {
	return g.runSilent("stash", "push", "-m", "ralph-auto-stash")
}

// StashPop restores the most recent stash entry.
func (g *Git) StashPop() error {
	return g.runSilent("stash", "pop")
}

// CurrentBranch returns the name of the checked-out branch.
func (g *Git) CurrentBranch() (string, error) {
	return g.run("rev-parse", "--abbrev-ref", "HEAD")
}

// HeadRef returns the full SHA of HEAD.
func (g *Git) HeadRef() (string, error) {
	return g.run("rev-parse", "HEAD")
}

// CommitsSince returns the number of commits between ref and HEAD.
func (g *Git) CommitsSince(ref string) (int, error) {
	out, err := g.run("rev-list", "--count", ref+"..HEAD")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(out)
}

// Push runs git push.
func (g *Git) Push() error {
	return g.runSilent("push")
}

// PushWithRetry attempts Push up to maxRetries times with 5 s between attempts.
func (g *Git) PushWithRetry(maxRetries int) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		if err = g.Push(); err == nil {
			return nil
		}
		if i < maxRetries-1 {
			time.Sleep(5 * time.Second)
		}
	}
	return err
}

// DiffStat returns the diff --stat output for the last commit.
func (g *Git) DiffStat() (string, error) {
	return g.run("diff", "--stat", "HEAD~1")
}
