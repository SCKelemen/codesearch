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
	"sync/atomic"
	"time"

	"github.com/SCKelemen/clix"
	"github.com/SCKelemen/codesearch"
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

	cmd.Run = func(ctx *clix.Context) error {
		ui := newCLIUI(ctx.App.Out)
		if concurrency < 1 {
			concurrency = maxConcurrentFetches
		}
		return runPierre(ctx, ui, baseURL, token, repoFilter, branch, outputDir, parseLanguageFilter(languageFilter), maxRepos, concurrency)
	}
	return cmd
}

func runPierre(ctx *clix.Context, ui *cliUI, baseURL, token, repoFilter, branch, outputDir string, langFilters map[string]struct{}, maxRepos, concurrency int) error {
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
	var totalFiles int
	var totalBytes int64
	var indexedRepos int

	for i, repo := range repos {
		ui.info("[%d/%d] %s", i+1, len(repos), repo.Name)

		ref := branch
		if ref == "" {
			ref = repo.DefaultBranch
		}
		if ref == "" {
			ref = "main"
		}

		files, bytes, err := indexPierreRepo(ctx, client, repo, ref, engine, langFilters, concurrency, ui)
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

// indexPierreRepo indexes a single Pierre repository. Tries the archive API
// first, falls back to parallel per-file fetching.
func indexPierreRepo(ctx *clix.Context, client *pierreClient, repo pierreRepo, ref string, engine *codesearch.Engine, langFilters map[string]struct{}, concurrency int, ui *cliUI) (int, int64, error) {
	files, bytes, err := indexPierreViaTarball(ctx, client, repo, ref, engine, langFilters, ui)
	if err == nil {
		return files, bytes, nil
	}

	ui.info("  archive unavailable (%v), using parallel file fetch", err)

	return indexPierreViaFiles(ctx, client, repo, ref, engine, langFilters, concurrency, ui)
}

// indexPierreViaTarball downloads the repo archive and indexes in a single pass.
func indexPierreViaTarball(ctx context.Context, client *pierreClient, repo pierreRepo, ref string, engine *codesearch.Engine, langFilters map[string]struct{}, ui *cliUI) (int, int64, error) {
	ui.info("  downloading archive for %s@%s...", repo.Name, ref)
	body, err := client.getArchive(ctx, repo.ID, ref)
	if err != nil {
		return 0, 0, err
	}
	defer body.Close()

	gz, err := gzip.NewReader(body)
	if err != nil {
		return 0, 0, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	tmpDir := filepath.Join(os.TempDir(), "csx-pierre-tarball", repo.ID)
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

		content, err := io.ReadAll(io.LimitReader(tr, maxFileSize+1))
		if err != nil {
			continue
		}
		if int64(len(content)) > maxFileSize {
			continue
		}

		tmpPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(tmpPath, content, 0o644); err != nil {
			continue
		}

		uri := fmt.Sprintf("pierre://%s/%s@%s", repo.Name, path, ref)
		if err := engine.Index(ctx, tmpPath, codesearch.WithURI(uri)); err != nil {
			continue
		}

		indexed++
		totalBytes += int64(len(content))
		if indexed == 1 || indexed%25 == 0 {
			ui.info("    tarball indexed %d files (%s)", indexed, humanBytes(totalBytes))
		}
	}

	return indexed, totalBytes, nil
}

// indexPierreViaFiles fetches files in parallel using the Pierre file API.
func indexPierreViaFiles(ctx context.Context, client *pierreClient, repo pierreRepo, ref string, engine *codesearch.Engine, langFilters map[string]struct{}, concurrency int, ui *cliUI) (int, int64, error) {
	files, err := client.listFiles(ctx, repo.ID, ref)
	if err != nil {
		return 0, 0, err
	}

	var entries []pierreFileEntry
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
		entries = append(entries, f)
	}

	if len(entries) == 0 {
		return 0, 0, fmt.Errorf("no indexable files")
	}

	ui.info("  fetching %d files via Pierre API...", len(entries))

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

			content, err := client.getFile(ctx, repo.ID, path, ref)
			if err != nil {
				fetchErrors.Add(1)
				return
			}
			if int64(len(content)) > maxFileSize {
				return
			}
			results <- result{path: path, content: content}
		}(entry.Path)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	tmpDir := filepath.Join(os.TempDir(), "csx-pierre-files", repo.ID)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	var indexed int
	var totalBytes int64
	var processed int

	for r := range results {
		processed++
		tmpPath := filepath.Join(tmpDir, r.path)
		if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(tmpPath, r.content, 0o644); err != nil {
			continue
		}

		uri := fmt.Sprintf("pierre://%s/%s@%s", repo.Name, r.path, ref)
		if err := engine.Index(ctx, tmpPath, codesearch.WithURI(uri)); err != nil {
			continue
		}

		indexed++
		totalBytes += int64(len(r.content))
		if processed == 1 || processed == len(entries) || processed%25 == 0 {
			ui.info("    indexed %d/%d files (%s)", processed, len(entries), humanBytes(totalBytes))
		}
	}

	if indexed == 0 && fetchErrors.Load() > 0 {
		return 0, 0, fmt.Errorf("failed to fetch %d files from Pierre", fetchErrors.Load())
	}

	return indexed, totalBytes, nil
}
