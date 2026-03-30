package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initRepo creates a bare-minimum git repo in dir with one empty commit.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
}

func TestIsDirty_Clean(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)
	g := New(dir)

	dirty, err := g.IsDirty()
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Error("expected clean repo to not be dirty")
	}
}

func TestIsDirty_Dirty(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)
	g := New(dir)

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirty, err := g.IsDirty()
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Error("expected dirty repo after untracked file")
	}
}

func TestCurrentBranch(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)
	g := New(dir)

	branch, err := g.CurrentBranch()
	if err != nil {
		t.Fatal(err)
	}
	// Default branch may be "master" or "main" depending on git config.
	if branch != "master" && branch != "main" {
		t.Errorf("unexpected branch %q", branch)
	}
}

func TestCommitsSince(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)
	g := New(dir)

	base, err := g.HeadRef()
	if err != nil {
		t.Fatal(err)
	}

	// Add two more commits.
	for i := 0; i < 2; i++ {
		cmd := exec.Command("git", "commit", "--allow-empty", "-m", "extra")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("commit failed: %v\n%s", err, out)
		}
	}

	count, err := g.CommitsSince(base)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 commits since base, got %d", count)
	}
}

func TestBatchPusher_Interval(t *testing.T) {
	// skipPush=true so we don't need a remote.
	g := New(t.TempDir())
	bp := NewBatchPusher(g, 3, true)

	for i := 1; i <= 5; i++ {
		pushed, err := bp.AfterIteration()
		if err != nil {
			t.Fatal(err)
		}
		expectPush := (i == 3) // push at iteration 3, counter resets
		if pushed != expectPush {
			t.Errorf("iteration %d: pushed=%v, want %v", i, pushed, expectPush)
		}
	}
}

func TestBatchPusher_Flush(t *testing.T) {
	g := New(t.TempDir())
	bp := NewBatchPusher(g, 10, true)

	// Accumulate some iterations without reaching the interval.
	for i := 0; i < 4; i++ {
		if _, err := bp.AfterIteration(); err != nil {
			t.Fatal(err)
		}
	}

	if err := bp.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// After flush the counter should be reset; a second flush is a no-op.
	if err := bp.Flush(); err != nil {
		t.Fatalf("second Flush failed: %v", err)
	}
}
