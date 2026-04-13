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
	"time"

	"github.com/SCKelemen/codesearch/pool"

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
	var shallow = true
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
	cmd.Flags.BoolVar(clix.BoolVarOptions{
		FlagOptions: clix.FlagOptions{Name: "shallow", Short: "s", Usage: "Shallow index: only the latest commit on the default branch (default)"},
		Value:       &shallow,
	})

	cmd.Run = func(ctx *clix.Context) error {
		ui := newCLIUI(ctx.App.Out)
		if concurrency < 1 {
			concurrency = maxConcurrentFetches
		}
		return runGitHub(ctx, ui, token, user, org, outputDir, parseLanguageFilter(languageFilter), maxRepos, concurrency, includeArchived, includeForked, shallow)
	}
	return cmd
}

func runGitHub(ctx *clix.Context, ui *cliUI, token, user, org, outputDir string, langFilters map[string]struct{}, maxRepos, concurrency int, includeArchived, includeForked, shallow bool) error {
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

	ui.info("discovering repositories...")
	repos, err := discoverRepos(ctx, client, user, org, maxRepos, includeArchived, includeForked, ui)
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

	// Map-reduce: download repos in parallel via pool, index serially.
	type repoDownload struct {
		repoName string
		files    []indexedEntry
		err      error
	}

	type githubIndexSummary struct {
		totalFiles     int
		totalBytes     int64
		indexedRepos   int
		skippedRepos   int
		processedRepos int
		downloadBytes  int64
		languages      map[string]int
	}

	summary, err := pool.MapReduce(ctx, repos, min(4, len(repos)),
		func(ctx context.Context, repo *github.Repository) (repoDownload, error) {
			entries, downloadErr := downloadGitHubRepo(ctx, client, repo, langFilters, concurrency)
			return repoDownload{repoName: repo.GetFullName(), files: entries, err: downloadErr}, nil
		},
		func(ctx context.Context, acc githubIndexSummary, dl repoDownload) (githubIndexSummary, error) {
			acc.processedRepos++
			ui.info("[%d/%d] %s", acc.processedRepos, len(repos), dl.repoName)
			if dl.err != nil {
				ui.info("  skip: %v", dl.err)
				return acc, nil
			}
			if len(dl.files) == 0 {
				ui.info("  skip: no indexable files")
				return acc, nil
			}

			repoFiles, repoBytes, indexErr := indexDownloadedFiles(ctx, engine, dl.files)
			if indexErr != nil {
				ui.info("  skip: %v", indexErr)
				return acc, nil
			}

			acc.totalFiles += repoFiles
			acc.totalBytes += repoBytes
			acc.indexedRepos++
			ui.info("  indexed %d files (%s)", repoFiles, humanBytes(repoBytes))
			return acc, nil
		},
		githubIndexSummary{},
	)
	if err != nil {
		return fmt.Errorf("index repos: %w", err)
	}

	if err := engine.Close(); err != nil {
		return fmt.Errorf("flush index: %w", err)
	}

	elapsed := time.Since(startedAt).Round(time.Millisecond)
	ui.println("")
	ui.section("Summary")
	if shallow {
		ui.kv("mode", "shallow (latest commit only)")
	} else {
		ui.kv("mode", "deep (all history)")
	}
	ui.kv("started", startedAt.Format(time.RFC3339))
	ui.kv("elapsed", elapsed.String())
	ui.kv("repos", fmt.Sprintf("%d indexed, %d skipped, %d total", summary.indexedRepos, summary.skippedRepos, len(repos)))
	ui.kv("files", fmt.Sprintf("%d", summary.totalFiles))
	ui.kv("content", humanBytes(summary.totalBytes))
	ui.kv("downloaded", humanBytes(summary.downloadBytes))
	if len(summary.languages) > 0 {
		ui.kv("languages", fmt.Sprintf("%d", len(summary.languages)))
	}
	ui.kv("output", resolvedOutput)
	ui.println("")
	ui.successf("indexed %d repos, %d files (%s) in %s", summary.indexedRepos, summary.totalFiles, humanBytes(summary.totalBytes), elapsed)
	ui.info("search with: csx search --index %s <query>", resolvedOutput)

	return nil
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

// indexedEntry holds a downloaded file ready for indexing.
type indexedEntry struct {
	uri     string
	content []byte
	tmpPath string
}

// downloadGitHubRepo downloads all files from a repo without indexing.
func downloadGitHubRepo(ctx context.Context, client *github.Client, repo *github.Repository, langFilters map[string]struct{}, concurrency int) ([]indexedEntry, error) {
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	branch := repo.GetDefaultBranch()
	if branch == "" {
		branch = "main"
	}

	entries, err := downloadViaTarball(ctx, client, owner, name, branch, langFilters)
	if err != nil {
		entries, err = downloadViaContentsAPI(ctx, client, owner, name, branch, langFilters, concurrency)
	}
	return entries, err
}

// downloadViaTarball downloads a repo tarball and extracts indexable files.
func downloadViaTarball(ctx context.Context, client *github.Client, owner, name, branch string, langFilters map[string]struct{}) ([]indexedEntry, error) {
	archiveURL, _, err := client.Repositories.GetArchiveLink(ctx, owner, name, github.Tarball, &github.RepositoryContentGetOptions{Ref: branch}, 10)
	if err != nil {
		return nil, fmt.Errorf("get archive link: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download tarball: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tarball HTTP %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var entries []indexedEntry

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return entries, fmt.Errorf("read tar: %w", err)
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

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

		fileContent, err := io.ReadAll(io.LimitReader(tr, maxFileSize+1))
		if err != nil {
			continue
		}
		if int64(len(fileContent)) > maxFileSize {
			continue
		}

		uri := fmt.Sprintf("github://%s/%s/%s@%s", owner, name, path, branch)
		entries = append(entries, indexedEntry{uri: uri, content: fileContent})
	}

	return entries, nil
}

// downloadViaContentsAPI downloads files in parallel using the GitHub Contents API.
func downloadViaContentsAPI(ctx context.Context, client *github.Client, owner, name, branch string, langFilters map[string]struct{}, concurrency int) ([]indexedEntry, error) {
	tree, _, err := client.Git.GetTree(ctx, owner, name, branch, true)
	if err != nil {
		return nil, fmt.Errorf("get tree: %w", err)
	}

	var paths []string
	for _, entry := range tree.Entries {
		if entry.GetType() != "blob" {
			continue
		}
		path := entry.GetPath()
		if skipPath(path) {
			continue
		}
		if entry.GetSize() > int(maxFileSize) {
			continue
		}
		if !languageAllowed(path, langFilters) {
			continue
		}
		paths = append(paths, path)
	}

	type result struct {
		entry indexedEntry
		err   error
	}

	sem := make(chan struct{}, concurrency)
	results := make(chan result, len(paths))
	var wg sync.WaitGroup

	for _, path := range paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fileContent, err := fetchFileContent(ctx, client, owner, name, p, branch)
			if err != nil {
				results <- result{err: err}
				return
			}
			uri := fmt.Sprintf("github://%s/%s/%s@%s", owner, name, p, branch)
			results <- result{entry: indexedEntry{uri: uri, content: fileContent}}
		}(path)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var entries []indexedEntry
	for r := range results {
		if r.err != nil {
			continue
		}
		entries = append(entries, r.entry)
	}
	return entries, nil
}

// indexDownloadedFiles indexes pre-downloaded files into the engine.
func indexDownloadedFiles(ctx context.Context, engine *codesearch.Engine, entries []indexedEntry) (int, int64, error) {
	tmpDir := filepath.Join(os.TempDir(), "csx-index-batch")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	var indexed int
	var totalBytes int64

	for _, entry := range entries {
		tmpPath := filepath.Join(tmpDir, fmt.Sprintf("f%d", indexed))
		if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(tmpPath, entry.content, 0o644); err != nil {
			continue
		}
		if err := engine.Index(ctx, tmpPath, codesearch.WithURI(entry.uri)); err != nil {
			continue
		}
		indexed++
		totalBytes += int64(len(entry.content))
	}

	return indexed, totalBytes, nil
}

func discoverRepos(ctx *clix.Context, client *github.Client, user, org string, maxRepos int, includeArchived, includeForked bool, ui *cliUI) ([]*github.Repository, error) {
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
		ui.info("  found %d repos so far, fetching page %d...", len(allRepos), resp.NextPage)
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
