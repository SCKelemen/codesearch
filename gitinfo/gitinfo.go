// Package gitinfo extracts repository and per-file metadata from a local
// .git directory using the git CLI. It never modifies the repository.
package gitinfo

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RepoInfo contains repository-level metadata extracted from .git.
type RepoInfo struct {
	// RemoteURL is the origin remote URL (e.g., git@github.com:user/repo.git).
	RemoteURL string
	// Repository is the derived repository name (e.g., "user/repo").
	Repository string
	// Branch is the current HEAD branch name.
	Branch string
	// DefaultBranch is the remote default branch (e.g., "main").
	DefaultBranch string
	// HeadCommit is the SHA of the current HEAD commit.
	HeadCommit string
	// Tags lists all tags pointing at HEAD.
	Tags []string
}

// FileInfo contains per-file metadata extracted from git log.
type FileInfo struct {
	// LastCommitSHA is the SHA of the most recent commit touching this file.
	LastCommitSHA string
	// LastAuthorName is the author name of the most recent commit.
	LastAuthorName string
	// LastAuthorEmail is the author email of the most recent commit.
	LastAuthorEmail string
	// LastCommitDate is the author date of the most recent commit.
	LastCommitDate time.Time
	// LastCommitMessage is the subject line of the most recent commit.
	LastCommitMessage string
	// CommitCount is the total number of commits that touched this file.
	CommitCount int
	// FirstCommitDate is the author date of the earliest commit touching this file.
	FirstCommitDate time.Time
}

// Repo reads repository-level metadata from the .git directory at repoRoot.
func Repo(ctx context.Context, repoRoot string) (*RepoInfo, error) {
	info := &RepoInfo{}

	// Current branch
	if branch, err := gitOutput(ctx, repoRoot, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		info.Branch = branch
	}

	// HEAD commit SHA
	if sha, err := gitOutput(ctx, repoRoot, "rev-parse", "HEAD"); err == nil {
		info.HeadCommit = sha
	}

	// Remote URL
	if url, err := gitOutput(ctx, repoRoot, "config", "--get", "remote.origin.url"); err == nil {
		info.RemoteURL = url
		info.Repository = repoNameFromURL(url)
	}

	// Default branch from remote HEAD
	if ref, err := gitOutput(ctx, repoRoot, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		// refs/remotes/origin/main -> main
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			info.DefaultBranch = parts[len(parts)-1]
		}
	}

	// Tags at HEAD
	if tagOutput, err := gitOutput(ctx, repoRoot, "tag", "--points-at", "HEAD"); err == nil && tagOutput != "" {
		for _, tag := range strings.Split(tagOutput, "\\n") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				info.Tags = append(info.Tags, tag)
			}
		}
	}

	return info, nil
}

// File reads per-file metadata from git log for the given file path.
// The path should be relative to repoRoot or absolute.
func File(ctx context.Context, repoRoot, filePath string) (*FileInfo, error) {
	relPath, err := filepath.Rel(repoRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	info := &FileInfo{}

	// Most recent commit: SHA, author, date, subject
	// Format: SHA|author name|author email|author date ISO|subject
	format := "%H|%an|%ae|%aI|%s"
	if line, err := gitOutput(ctx, repoRoot, "log", "-1", "--format="+format, "--", relPath); err == nil && line != "" {
		parts := strings.SplitN(line, "|", 5)
		if len(parts) == 5 {
			info.LastCommitSHA = parts[0]
			info.LastAuthorName = parts[1]
			info.LastAuthorEmail = parts[2]
			if t, err := time.Parse(time.RFC3339, parts[3]); err == nil {
				info.LastCommitDate = t
			}
			info.LastCommitMessage = parts[4]
		}
	}

	// Commit count
	if countStr, err := gitOutput(ctx, repoRoot, "rev-list", "--count", "HEAD", "--", relPath); err == nil {
		var count int
		if _, err := fmt.Sscanf(countStr, "%d", &count); err == nil {
			info.CommitCount = count
		}
	}

	// First commit date
	if line, err := gitOutput(ctx, repoRoot, "log", "--reverse", "--format=%aI", "-1", "--", relPath); err == nil && line != "" {
		if t, err := time.Parse(time.RFC3339, line); err == nil {
			info.FirstCommitDate = t
		}
	}

	return info, nil
}

// BatchFileInfo retrieves per-file metadata for multiple files efficiently.
// It runs a single git log per file but limits concurrency.
func BatchFileInfo(ctx context.Context, repoRoot string, filePaths []string) (map[string]*FileInfo, error) {
	results := make(map[string]*FileInfo, len(filePaths))
	for _, path := range filePaths {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		info, err := File(ctx, repoRoot, path)
		if err != nil {
			continue // skip files with git errors
		}
		results[path] = info
	}
	return results, nil
}

func gitOutput(ctx context.Context, repoRoot string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// repoNameFromURL extracts "owner/repo" from various git remote URL formats.
func repoNameFromURL(rawURL string) string {
	// SSH: git@github.com:owner/repo.git
	if strings.HasPrefix(rawURL, "git@") {
		if idx := strings.Index(rawURL, ":"); idx >= 0 {
			path := rawURL[idx+1:]
			path = strings.TrimSuffix(path, ".git")
			return path
		}
	}
	// HTTPS: https://github.com/owner/repo.git
	rawURL = strings.TrimSuffix(rawURL, ".git")
	parts := strings.Split(rawURL, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return rawURL
}
