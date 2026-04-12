package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SCKelemen/clix"
	"github.com/SCKelemen/codesearch"
	"github.com/SCKelemen/codesearch/structural"
)

func newIndexCommand() *clix.Command {
	cmd := clix.NewCommand("index")
	cmd.Short = "Index a file or directory into a local engine store"
	cmd.Usage = "csx index <path> [--output ./index] [--language go,ts] [--embeddings]"
	cmd.Arguments = []*clix.Argument{{
		Name:     "path",
		Prompt:   "Path to index",
		Required: true,
	}}

	var outputDir string
	var languageFilter string
	var embeddings bool

	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "output", Short: "o", Usage: "Directory that will hold the generated index"},
		Default:     defaultIndexDir,
		Value:       &outputDir,
	})
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "language", Short: "l", Usage: "Comma-separated language filter (for example: go,ts,typescript)"},
		Value:       &languageFilter,
	})
	cmd.Flags.BoolVar(clix.BoolVarOptions{
		FlagOptions: clix.FlagOptions{Name: "embeddings", Usage: "Build deterministic local semantic embeddings"},
		Value:       &embeddings,
	})

	cmd.Run = func(ctx *clix.Context) error {
		ui := newCLIUI(ctx.App.Out)
		return runIndex(ctx, ui, ctx.Args[0], outputDir, parseLanguageFilter(languageFilter), embeddings, useLSP)
	}
	cmd.PostRun = func(ctx *clix.Context) error {
		_, _ = fmt.Fprintln(ctx.App.Out)
		return nil
	}
	return cmd
}

func runIndex(ctx *clix.Context, ui *cliUI, rootPath, outputDir string, filters map[string]struct{}, embeddings bool, useLSP bool) error {
	resolvedRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("resolve input path: %w", err)
	}
	resolvedOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}
	inputs, err := collectIndexInputs(resolvedRoot, resolvedOutput, filters)
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return fmt.Errorf("no indexable files found under %s", resolvedRoot)
	}
	if err := os.MkdirAll(resolvedOutput, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	workDir := resolvedRoot
	if info, err := os.Stat(resolvedRoot); err == nil && !info.IsDir() {
		workDir = filepath.Dir(resolvedRoot)
	}

	mux := setupLSP(ctx, workDir, useLSP)
	defer func() {
		if mux != nil {
			_ = mux.Close()
		}
	}()

	engine, err := openEngine(resolvedOutput)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer func() {
		_ = engine.Close()
	}()

	ui.section("Indexing")
	ui.info("input %s", resolvedRoot)
	ui.info("output %s", resolvedOutput)
	if len(filters) > 0 {
		ui.info("languages %s", strings.Join(sortedKeys(filters), ", "))
	}
	ui.info("embeddings %t", embeddings)
	ui.info("lsp %t", mux != nil)
	ui.info("files %d", len(inputs))

	var lsifStatsText string
	if mux != nil {
		lsifPath := filepath.Join(resolvedOutput, defaultLSIFFile)
		stats, err := writeLSIF(ctx, mux, inputs, lsifPath)
		if err != nil {
			return fmt.Errorf("generate LSIF: %w", err)
		}
		ui.info("lsif %s", lsifPath)
		lsifStatsText = formatLSIFStats(stats)
	}

	indexOpts := []codesearch.IndexOption{codesearch.WithEmbeddings(embeddings)}
	if mux != nil {
		indexOpts = append(indexOpts, codesearch.WithSymbolExtractor(func(ctx context.Context, path, language string, content []byte) ([]structural.Symbol, error) {
			return structural.ExtractWithLSP(ctx, mux, path, content)
		}))
	}

	startedAt := time.Now()
	var indexedBytes int64
	for i, input := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := engine.Index(ctx, input, indexOpts...); err != nil {
			return fmt.Errorf("index %s: %w", input, err)
		}
		if info, err := os.Stat(input); err == nil {
			indexedBytes += info.Size()
		}
		count := i + 1
		if count == 1 || count == len(inputs) || count%25 == 0 {
			ui.info("indexed %d/%d files in %s", count, len(inputs), time.Since(startedAt).Round(time.Millisecond))
		}
	}
	if err := engine.Close(); err != nil {
		return fmt.Errorf("flush index: %w", err)
	}
	ui.successf("indexed %d files (%s) in %s", len(inputs), humanBytes(indexedBytes), time.Since(startedAt).Round(time.Millisecond))
	if lsifStatsText != "" {
		ui.info("%s", lsifStatsText)
	}
	return nil
}

func collectIndexInputs(rootPath, outputDir string, filters map[string]struct{}) ([]string, error) {
	info, err := os.Stat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", rootPath, err)
	}
	if !info.IsDir() {
		if !languageAllowed(rootPath, filters) {
			return nil, nil
		}
		binary, err := isBinaryPath(rootPath)
		if err != nil {
			return nil, err
		}
		if binary {
			return nil, nil
		}
		return []string{rootPath}, nil
	}

	inputs := make([]string, 0, 256)
	err = filepath.WalkDir(rootPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == outputDir {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !languageAllowed(path, filters) {
			return nil
		}
		binary, err := isBinaryPath(path)
		if err != nil {
			return err
		}
		if binary {
			return nil
		}
		inputs = append(inputs, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", rootPath, err)
	}
	sort.Strings(inputs)
	return inputs, nil
}

func isBinaryPath(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	buffer := make([]byte, 1024)
	count, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	for _, b := range buffer[:count] {
		if b == 0 {
			return true, nil
		}
	}
	return false, nil
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
