package gitinfo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRepoNameFromURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url  string
		want string
	}{
		{"git@github.com:SCKelemen/codesearch.git", "SCKelemen/codesearch"},
		{"https://github.com/SCKelemen/codesearch.git", "SCKelemen/codesearch"},
		{"https://github.com/SCKelemen/codesearch", "SCKelemen/codesearch"},
		{"git@gitlab.com:org/project.git", "org/project"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()
			got := repoNameFromURL(tt.url)
			if got != tt.want {
				t.Fatalf("repoNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestRepoOnRealRepo(t *testing.T) {
	t.Parallel()
	// This test runs against the actual codesearch repo
	repoRoot := findRepoRoot(t)
	ctx := context.Background()

	info, err := Repo(ctx, repoRoot)
	if err != nil {
		t.Fatalf("Repo() error: %v", err)
	}
	if info.Branch == "" {
		t.Error("expected non-empty branch")
	}
	if info.HeadCommit == "" {
		t.Error("expected non-empty head commit")
	}
	if len(info.HeadCommit) != 40 {
		t.Errorf("expected 40-char SHA, got %d: %q", len(info.HeadCommit), info.HeadCommit)
	}
	if info.Repository == "" {
		t.Error("expected non-empty repository name")
	}
	t.Logf("repo: %s, branch: %s, head: %s", info.Repository, info.Branch, info.HeadCommit[:8])
}

func TestFileOnRealRepo(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	ctx := context.Background()

	// Test against a file we know exists
	filePath := filepath.Join(repoRoot, "go.mod")
	info, err := File(ctx, repoRoot, filePath)
	if err != nil {
		t.Fatalf("File() error: %v", err)
	}
	if info.LastCommitSHA == "" {
		t.Error("expected non-empty last commit SHA")
	}
	if info.LastAuthorName == "" {
		t.Error("expected non-empty last author name")
	}
	if info.LastCommitDate.IsZero() {
		t.Error("expected non-zero last commit date")
	}
	if info.CommitCount <= 0 {
		t.Error("expected positive commit count")
	}
	if info.FirstCommitDate.IsZero() {
		t.Error("expected non-zero first commit date")
	}
	if !info.FirstCommitDate.Before(info.LastCommitDate) && !info.FirstCommitDate.Equal(info.LastCommitDate) {
		t.Errorf("first commit date %v should be <= last commit date %v", info.FirstCommitDate, info.LastCommitDate)
	}
	t.Logf("go.mod: author=%s, commits=%d, last=%s", info.LastAuthorName, info.CommitCount, info.LastCommitDate.Format(time.RFC3339))
}

func TestBatchFileInfo(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	ctx := context.Background()

	files := []string{
		filepath.Join(repoRoot, "go.mod"),
		filepath.Join(repoRoot, "go.sum"),
		filepath.Join(repoRoot, "nonexistent-file-that-wont-exist.xyz"),
	}
	results, err := BatchFileInfo(ctx, repoRoot, files)
	if err != nil {
		t.Fatalf("BatchFileInfo() error: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("expected at least 2 results, got %d", len(results))
	}
}

func TestRepoOnTempRepo(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	ctx := context.Background()

	// Initialize a temp git repo
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test Author",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test Author",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial commit")

	info, err := Repo(ctx, dir)
	if err != nil {
		t.Fatalf("Repo() error: %v", err)
	}
	if info.Branch != "main" {
		t.Errorf("expected branch main, got %q", info.Branch)
	}
	if info.HeadCommit == "" {
		t.Error("expected non-empty head commit")
	}

	fileInfo, err := File(ctx, dir, filepath.Join(dir, "hello.go"))
	if err != nil {
		t.Fatalf("File() error: %v", err)
	}
	if fileInfo.LastAuthorName != "Test Author" {
		t.Errorf("expected author 'Test Author', got %q", fileInfo.LastAuthorName)
	}
	if fileInfo.CommitCount != 1 {
		t.Errorf("expected 1 commit, got %d", fileInfo.CommitCount)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("not inside a git repository")
		}
		dir = parent
	}
}
