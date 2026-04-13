package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
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
)

const defaultPierreIndexDir = ".csx/pierre"

// pierreClient wraps the Pierre/code.storage HTTP API.
type pierreClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func newPierreClient(baseURL, token string) *pierreClient {
	return &pierreClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *pierreClient) do(ctx context.Context, method, path string) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.httpClient.Do(req)
}

type pierreRepo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
	Description   string `json:"description"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type pierreBranch struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

type pierreFileEntry struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Mode     string `json:"mode"`
	Type     string `json:"type"`
	Language string `json:"language"`
}

// listRepos calls GET /api/v1/repos
func (c *pierreClient) listRepos(ctx context.Context) ([]pierreRepo, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/v1/repos")
	if err != nil {
		return nil, fmt.Errorf("list repos: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("list repos: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var repos []pierreRepo
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, fmt.Errorf("decode repos: %w", err)
	}
	return repos, nil
}

// listBranches calls GET /api/v1/repos/{repo}/branches
func (c *pierreClient) listBranches(ctx context.Context, repoID string) ([]pierreBranch, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/v1/repos/"+repoID+"/branches")
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("list branches: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var branches []pierreBranch
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		return nil, fmt.Errorf("decode branches: %w", err)
	}
	return branches, nil
}

// listFiles calls GET /api/v1/repos/{repo}/files?ref={ref}
func (c *pierreClient) listFiles(ctx context.Context, repoID, ref string) ([]pierreFileEntry, error) {
	path := "/api/v1/repos/" + repoID + "/files"
	if ref != "" {
		path += "?ref=" + ref
	}
	resp, err := c.do(ctx, http.MethodGet, path)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("list files: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var files []pierreFileEntry
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("decode files: %w", err)
	}
	return files, nil
}

// getFile calls GET /api/v1/repos/{repo}/file/{path}?ref={ref}
func (c *pierreClient) getFile(ctx context.Context, repoID, filePath, ref string) ([]byte, error) {
	path := "/api/v1/repos/" + repoID + "/file/" + filePath
	if ref != "" {
		path += "?ref=" + ref
	}
	resp, err := c.do(ctx, http.MethodGet, path)
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get file %s: HTTP %d", filePath, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxFileSize+1))
}

// getArchive calls GET /api/v1/repos/{repo}/archive?ref={ref}
func (c *pierreClient) getArchive(ctx context.Context, repoID, ref string) (io.ReadCloser, error) {
	path := "/api/v1/repos/" + repoID + "/archive"
	if ref != "" {
		path += "?ref=" + ref
	}
	resp, err := c.do(ctx, http.MethodGet, path)
	if err != nil {
		return nil, fmt.Errorf("get archive: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("get archive: HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func newPierreCommand() *clix.Command {
	cmd := clix.NewCommand("pierre")
	cmd.Short = "Index repositories from a Pierre/code.storage instance"
	cmd.Usage = "csx pierre [--url URL] [--token TOKEN] [--repo REPO] [--output DIR] [--language go,ts]"

	var baseURL string
	var token string
	var repoFilter string
	var outputDir string
	var languageFilter string
	var branch string
	var maxRepos int
	var concurrency int
	var shallow = true

	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "url", Usage: "Pierre instance URL (or set PIERRE_URL)"},
		Value:       &baseURL,
	})
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "token", Short: "t", Usage: "Pierre API token (or set PIERRE_TOKEN)"},
		Value:       &token,
	})
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "repo", Short: "r", Usage: "Index a specific repo by ID or name (default: all)"},
		Value:       &repoFilter,
	})
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "branch", Short: "b", Usage: "Branch to index (default: repo default branch)"},
		Value:       &branch,
	})
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "output", Short: "o", Usage: "Directory for local indexes"},
		Default:     defaultPierreIndexDir,
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
		FlagOptions: clix.FlagOptions{Name: "shallow", Short: "s", Usage: "Shallow index: only the latest commit on the default branch (default)"},
		Value:       &shallow,
	})

	cmd.Run = func(ctx *clix.Context) error {
		ui := newCLIUI(ctx.App.Out)
		if concurrency < 1 {
			concurrency = maxConcurrentFetches
		}
		return runPierre(ctx, ui, baseURL, token, repoFilter, branch, outputDir, parseLanguageFilter(languageFilter), maxRepos, concurrency, shallow)
	}
	return cmd
}

func runPierre(ctx *clix.Context, ui *cliUI, baseURL, token, repoFilter, branch, outputDir string, langFilters map[string]struct{}, maxRepos, concurrency int, shallow bool) error {
	if baseURL == "" {
		baseURL = os.Getenv("PIERRE_URL")
	}
	if baseURL == "" {
		return fmt.Errorf("Pierre URL required: use --url or set PIERRE_URL")
	}
	if token == "" {
		token = os.Getenv("PIERRE_TOKEN")
	}

	client := newPierreClient(baseURL, token)

	ui.section("Pierre Code Search Indexer")
	ui.info("discovering repositories...")

	repos, err := discoverPierreRepos(ctx, client, repoFilter, maxRepos)
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

	// Pipeline: Source -> Download (parallel) -> Index (serial sink)
	type pierreDownload struct {
		repoName string
		files    []indexedEntry
		err      error
	}

	repoQueue := pool.NewQueue[pierreRepo](len(repos))
	downloadQueue := pool.NewQueue[pierreDownload](concurrency)

	// Stage 1: Push discovered repos onto the queue
	pool.Source(ctx, repos, repoQueue)

	// Stage 2: Download repos in parallel
	pool.RunStage(ctx, pool.Stage[pierreRepo, pierreDownload]{
		Name:        "download",
		Concurrency: min(concurrency, len(repos)),
		Process: func(ctx context.Context, repo pierreRepo) (pierreDownload, error) {
			ref := branch
			if ref == "" {
				ref = repo.DefaultBranch
			}
			if ref == "" {
				ref = "main"
			}
			entries, downloadErr := downloadPierreRepo(ctx, client, repo, ref, langFilters, concurrency)
			return pierreDownload{repoName: repo.Name, files: entries, err: downloadErr}, nil
		},
	}, repoQueue, downloadQueue)

	// Stage 3: Index serially as downloads arrive (sink)
	var totalFiles int
	var totalBytes int64
	var indexedRepos int
	var processedRepos int

	sinkErr := pool.Sink(ctx, downloadQueue, func(ctx context.Context, dl pierreDownload) error {
		processedRepos++
		ui.info("[%d/%d] %s", processedRepos, len(repos), dl.repoName)
		if dl.err != nil {
			ui.info("  skip: %v", dl.err)
			return nil
		}
		if len(dl.files) == 0 {
			ui.info("  skip: no indexable files")
			return nil
		}

		repoFiles, repoBytes, indexErr := indexDownloadedFiles(ctx, engine, dl.files)
		if indexErr != nil {
			ui.info("  skip: %v", indexErr)
			return nil
		}

		totalFiles += repoFiles
		totalBytes += repoBytes
		indexedRepos++
		ui.info("  indexed %d files (%s)", repoFiles, humanBytes(repoBytes))
		return nil
	})
	if sinkErr != nil {
		return fmt.Errorf("index repos: %w", sinkErr)
	}

	if err := engine.Close(); err != nil {
		return fmt.Errorf("flush index: %w", err)
	}

	elapsed := time.Since(startedAt).Round(time.Millisecond)
	skippedRepos := len(repos) - indexedRepos
	ui.println("")
	ui.section("Summary")
	if shallow {
		ui.kv("mode", "shallow (latest commit only)")
	} else {
		ui.kv("mode", "deep (all history)")
	}
	ui.kv("started", startedAt.Format(time.RFC3339))
	ui.kv("elapsed", elapsed.String())
	ui.kv("repos", fmt.Sprintf("%d indexed, %d skipped, %d total", indexedRepos, skippedRepos, len(repos)))
	ui.kv("files", fmt.Sprintf("%d", totalFiles))
	ui.kv("content", humanBytes(totalBytes))
	ui.kv("output", resolvedOutput)
	ui.println("")
	ui.successf("indexed %d repos, %d files (%s) in %s", indexedRepos, totalFiles, humanBytes(totalBytes), elapsed)
	ui.info("search with: csx search --index %s <query>", resolvedOutput)

	return nil
}

// downloadPierreRepo downloads all files from a Pierre repo without indexing.
func downloadPierreRepo(ctx context.Context, client *pierreClient, repo pierreRepo, ref string, langFilters map[string]struct{}, concurrency int) ([]indexedEntry, error) {
	entries, err := downloadPierreTarball(ctx, client, repo, ref, langFilters)
	if err != nil {
		entries, err = downloadPierreFiles(ctx, client, repo, ref, langFilters, concurrency)
	}
	return entries, err
}

// downloadPierreTarball downloads a Pierre repo archive and extracts files.
func downloadPierreTarball(ctx context.Context, client *pierreClient, repo pierreRepo, ref string, langFilters map[string]struct{}) ([]indexedEntry, error) {
	body, err := client.getArchive(ctx, repo.ID, ref)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	gz, err := gzip.NewReader(body)
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
			path = hdr.Name
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

		uri := fmt.Sprintf("pierre://%s/%s@%s", repo.Name, path, ref)
		entries = append(entries, indexedEntry{uri: uri, content: fileContent})
	}

	return entries, nil
}

// downloadPierreFiles downloads files in parallel using the Pierre file API.
func downloadPierreFiles(ctx context.Context, client *pierreClient, repo pierreRepo, ref string, langFilters map[string]struct{}, concurrency int) ([]indexedEntry, error) {
	files, err := client.listFiles(ctx, repo.ID, ref)
	if err != nil {
		return nil, err
	}

	var paths []pierreFileEntry
	for _, f := range files {
		if f.Type != "blob" && f.Type != "file" && f.Type != "" {
			continue
		}
		if f.Size > maxFileSize {
			continue
		}
		if skipPath(f.Path) {
			continue
		}
		if !languageAllowed(f.Path, langFilters) {
			continue
		}
		paths = append(paths, f)
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("no indexable files")
	}

	type result struct {
		entry indexedEntry
		err   error
	}

	sem := make(chan struct{}, concurrency)
	results := make(chan result, len(paths))
	var wg sync.WaitGroup

	for _, entry := range paths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fileContent, err := client.getFile(ctx, repo.ID, path, ref)
			if err != nil {
				results <- result{err: err}
				return
			}
			uri := fmt.Sprintf("pierre://%s/%s@%s", repo.Name, path, ref)
			results <- result{entry: indexedEntry{uri: uri, content: fileContent}}
		}(entry.Path)
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

func discoverPierreRepos(ctx *clix.Context, client *pierreClient, repoFilter string, maxRepos int) ([]pierreRepo, error) {
	allRepos, err := client.listRepos(ctx)
	if err != nil {
		return nil, err
	}

	if repoFilter != "" {
		var filtered []pierreRepo
		for _, repo := range allRepos {
			if repo.ID == repoFilter || repo.Name == repoFilter {
				filtered = append(filtered, repo)
			}
		}
		return filtered, nil
	}

	if maxRepos > 0 && len(allRepos) > maxRepos {
		allRepos = allRepos[:maxRepos]
	}

	return allRepos, nil
}
