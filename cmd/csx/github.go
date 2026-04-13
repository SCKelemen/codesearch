package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SCKelemen/clix"
	"github.com/SCKelemen/codesearch"
	"github.com/google/go-github/v72/github"
)

const (
	defaultGitHubIndexDir = ".csx/github"
	maxConcurrentFetches  = 8
	maxFileSize           = 1 << 20 // 1 MiB
)

func newGitHubCommand() *clix.Command {
	cmd := clix.NewCommand("github")
	cmd.Short = "Index your GitHub repositories for local code search"
	cmd.Usage = "csx github [--token PAT] [--user USER] [--org ORG] [--output DIR] [--language go,ts]"

	var token string
	var user string
	var org string
	var outputDir string
	var languageFilter string
	var maxRepos int
	var includeArchived bool
	var includeForked bool
	var concurrency int

	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "token", Short: "t", Usage: "GitHub personal access token (or set GITHUB_TOKEN)"},
		Value:       &token,
	})
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "user", Short: "u", Usage: "GitHub username (defaults to authenticated user)"},
		Value:       &user,
	})
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "org", Usage: "GitHub organization to index"},
		Value:       &org,
	})
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "output", Short: "o", Usage: "Directory for local indexes"},
		Default:     defaultGitHubIndexDir,
		Value:       &outputDir,
	})
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "language", Short: "l", Usage: "Comma-separated language filter"},
		Value:       &languageFilter,
	})
	cmd.Flags.IntVar(clix.IntVarOptions{
		FlagOptions: clix.FlagOptions{Name: "max-repos", Usage: "Maximum number of repos to index (0 = all)"},
		Value:       &maxRepos,
	})
	cmd.Flags.IntVar(clix.IntVarOptions{
		FlagOptions: clix.FlagOptions{Name: "concurrency", Short: "c", Usage: "Parallel file fetches for fallback mode (default 8)"},
		Default:     "8",
		Value:       &concurrency,
	})
	cmd.Flags.BoolVar(clix.BoolVarOptions{
		FlagOptions: clix.FlagOptions{Name: "archived", Usage: "Include archived repositories"},
		Value:       &includeArchived,
	})
	cmd.Flags.BoolVar(clix.BoolVarOptions{
		FlagOptions: clix.FlagOptions{Name: "forks", Usage: "Include forked repositories"},
		Value:       &includeForked,
	})

	cmd.Run = func(ctx *clix.Context) error {
		ui := newCLIUI(ctx.App.Out)
		if concurrency < 1 {
			concurrency = maxConcurrentFetches
		}
		return runGitHub(ctx, ui, token, user, org, outputDir, parseLanguageFilter(languageFilter), maxRepos, concurrency, includeArchived, includeForked)
	}
	return cmd
}

func runGitHub(ctx *clix.Context, ui *cliUI, token, user, org, outputDir string, langFilters map[string]struct{}, maxRepos, concurrency int, includeArchived, includeForked bool) error {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		return fmt.Errorf("GitHub token required: use --token or set GITHUB_TOKEN / GH_TOKEN")
	}

	client := github.NewClient(nil).WithAuthToken(token)

	ui.section("GitHub Code Search Indexer")

	repos, err := discoverRepos(ctx, client, user, org, maxRepos, includeArchived, includeForked)
	if err != nil {
		return fmt.Errorf("discover repos: %w", err)
	}
	if len(repos) == 0 {
		return fmt.Errorf("no repositories found")
	}

	ui.info("found %d repositories", len(repos))

	resolvedOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}
	if err := os.MkdirAll(resolvedOutput, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	engine, err := openEngine(resolvedOutput)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}

	startedAt := time.Now()
	var totalFiles int
	var totalBytes int64
	var indexedRepos int

	for i, repo := range repos {
		repoName := repo.GetFullName()
		ui.info("[%d/%d] %s", i+1, len(repos), repoName)

		files, bytes, err := indexGitHubRepo(ctx, client, repo, engine, langFilters, concurrency, ui)
		if err != nil {
			ui.info("  skip: %v", err)
			continue
		}

		totalFiles += files
		totalBytes += bytes
		indexedRepos++
		ui.info("  indexed %d files (%s)", files, humanBytes(bytes))
	}

	if err := engine.Close(); err != nil {
		return fmt.Errorf("flush index: %w", err)
	}

	elapsed := time.Since(startedAt).Round(time.Millisecond)
	ui.successf("indexed %d repos, %d files (%s) in %s", indexedRepos, totalFiles, humanBytes(totalBytes), elapsed)
	ui.info("indexes stored in %s", resolvedOutput)
	ui.info("")
	ui.info("search with: csx search --index %s <query>", resolvedOutput)

	return nil
}

// indexGitHubRepo indexes a single repository. It tries the tarball API first
// (single HTTP request for the whole repo), falling back to parallel per-file
// fetching via the Contents API if the tarball is unavailable.
func indexGitHubRepo(ctx *clix.Context, client *github.Client, repo *github.Repository, engine *codesearch.Engine, langFilters map[string]struct{}, concurrency int, ui *cliUI) (int, int64, error) {
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	branch := repo.GetDefaultBranch()
	if branch == "" {
		branch = "main"
	}

	// Try tarball first — one HTTP request for the entire repo
	files, bytes, err := indexViaTarball(ctx, client, owner, name, branch, engine, langFilters, ui)
	if err == nil {
		return files, bytes, nil
	}

	ui.info("  tarball unavailable (%v), using parallel API fetch", err)

	// Fallback: parallel per-file fetch via Contents API
	return indexViaContentsAPI(ctx, client, owner, name, branch, engine, langFilters, concurrency)
}

// indexViaTarball downloads the repo as a gzipped tarball and indexes all files
// in a single pass. This uses 1 API call regardless of repo size.
func indexViaTarball(ctx context.Context, client *github.Client, owner, name, branch string, engine *codesearch.Engine, langFilters map[string]struct{}, ui *cliUI) (int, int64, error) {
	archiveURL, _, err := client.Repositories.GetArchiveLink(ctx, owner, name, github.Tarball, &github.RepositoryContentGetOptions{Ref: branch}, 10)
	if err != nil {
		return 0, 0, fmt.Errorf("get archive link: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL.String(), nil)
	if err != nil {
		return 0, 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("download tarball: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("tarball HTTP %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return 0, 0, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	tmpDir := filepath.Join(os.TempDir(), "csx-tarball", owner, name)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	var indexed int
	var totalBytes int64

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return indexed, totalBytes, fmt.Errorf("read tar: %w", err)
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		// GitHub tarballs have a prefix like "owner-repo-sha/"
		// Strip the first path component to get the real file path
		path := stripTarPrefix(hdr.Name)
		if path == "" {
			continue
		}

		if hdr.Size > maxFileSize {
			continue
		}
		if skipPath(path) {
			continue
		}
		if !languageAllowed(path, langFilters) {
			continue
		}

		content, err := io.ReadAll(io.LimitReader(tr, maxFileSize+1))
		if err != nil {
			continue
		}
		if int64(len(content)) > maxFileSize {
			continue
		}

		// Write to temp file for the engine
		tmpPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(tmpPath, content, 0o644); err != nil {
			continue
		}

		uri := fmt.Sprintf("github://%s/%s/%s@%s", owner, name, path, branch)
		if err := engine.Index(ctx, tmpPath, codesearch.WithURI(uri)); err != nil {
			continue
		}

		indexed++
		totalBytes += int64(len(content))
	}

	return indexed, totalBytes, nil
}

// stripTarPrefix removes the first path component from a tar entry name.
// GitHub tarballs use "owner-repo-commitsha/" as the prefix.
func stripTarPrefix(name string) string {
	idx := strings.IndexByte(name, '/')
	if idx < 0 || idx == len(name)-1 {
		return ""
	}
	return name[idx+1:]
}

// indexViaContentsAPI fetches files in parallel using the GitHub Contents API.
// This is the fallback when tarball download is unavailable.
func indexViaContentsAPI(ctx context.Context, client *github.Client, owner, name, branch string, engine *codesearch.Engine, langFilters map[string]struct{}, concurrency int) (int, int64, error) {
	tree, _, err := client.Git.GetTree(ctx, owner, name, branch, true)
	if err != nil {
		return 0, 0, fmt.Errorf("get tree: %w", err)
	}

	// Filter to indexable files
	var entries []*github.TreeEntry
	for _, entry := range tree.Entries {
		if entry.GetType() != "blob" {
			continue
		}
		path := entry.GetPath()
		if entry.GetSize() > maxFileSize {
			continue
		}
		if skipPath(path) {
			continue
		}
		if !languageAllowed(path, langFilters) {
			continue
		}
		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return 0, 0, fmt.Errorf("no indexable files")
	}

	// Parallel fetch with bounded concurrency
	type result struct {
		path    string
		content []byte
	}

	sem := make(chan struct{}, concurrency)
	results := make(chan result, len(entries))
	var wg sync.WaitGroup
	var fetchErrors atomic.Int64

	for _, entry := range entries {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			content, err := fetchFileContent(ctx, client, owner, name, path, branch)
			if err != nil {
				fetchErrors.Add(1)
				return
			}
			results <- result{path: path, content: content}
		}(entry.GetPath())
	}

	// Close results channel when all fetches complete
	go func() {
		wg.Wait()
		close(results)
	}()

	tmpDir := filepath.Join(os.TempDir(), "csx-github-tmp", owner, name)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	var indexed int
	var totalBytes int64

	for r := range results {
		tmpPath := filepath.Join(tmpDir, r.path)
		if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(tmpPath, r.content, 0o644); err != nil {
			continue
		}

		uri := fmt.Sprintf("github://%s/%s/%s@%s", owner, name, r.path, branch)
		if err := engine.Index(ctx, tmpPath, codesearch.WithURI(uri)); err != nil {
			continue
		}

		indexed++
		totalBytes += int64(len(r.content))
	}

	return indexed, totalBytes, nil
}

func discoverRepos(ctx *clix.Context, client *github.Client, user, org string, maxRepos int, includeArchived, includeForked bool) ([]*github.Repository, error) {
	var allRepos []*github.Repository
	opts := &github.RepositoryListOptions{
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		var repos []*github.Repository
		var resp *github.Response
		var err error

		if org != "" {
			orgOpts := &github.RepositoryListByOrgOptions{
				Sort:        "updated",
				Direction:   "desc",
				ListOptions: opts.ListOptions,
			}
			repos, resp, err = client.Repositories.ListByOrg(ctx, org, orgOpts)
		} else if user != "" {
			repos, resp, err = client.Repositories.List(ctx, user, opts)
		} else {
			repos, resp, err = client.Repositories.List(ctx, "", opts)
		}
		if err != nil {
			return nil, fmt.Errorf("list repos: %w", err)
		}

		for _, repo := range repos {
			if repo.GetArchived() && !includeArchived {
				continue
			}
			if repo.GetFork() && !includeForked {
				continue
			}
			allRepos = append(allRepos, repo)
			if maxRepos > 0 && len(allRepos) >= maxRepos {
				return allRepos, nil
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allRepos, nil
}

func fetchFileContent(ctx context.Context, client *github.Client, owner, name, path, branch string) ([]byte, error) {
	opts := &github.RepositoryContentGetOptions{Ref: branch}
	fileContent, _, resp, err := client.Repositories.GetContents(ctx, owner, name, path, opts)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if fileContent == nil {
		return nil, fmt.Errorf("nil content")
	}
	content, err := fileContent.GetContent()
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

func skipPath(path string) bool {
	parts := strings.Split(path, "/")
	for _, part := range parts {
		switch part {
		case ".git", "node_modules", "vendor", "dist", "build",
			".next", "__pycache__", ".tox", "target",
			".idea", ".vscode", ".gradle":
			return true
		}
	}
	base := filepath.Base(path)
	switch {
	case base == "go.sum",
		base == "package-lock.json",
		base == "yarn.lock",
		base == "pnpm-lock.yaml",
		base == "Cargo.lock",
		base == "poetry.lock",
		strings.HasSuffix(base, ".min.js"),
		strings.HasSuffix(base, ".min.css"),
		strings.HasSuffix(base, ".map"):
		return true
	}
	return false
}
