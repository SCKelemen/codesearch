package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SCKelemen/codesearch/lsp"
	"github.com/SCKelemen/codesearch/lsp/lsifgen"
)

const defaultLSIFFile = "index.lsif"

// setupLSP starts language servers if --lsp is enabled.
// Returns a *lsp.Multiplexer (or nil if --lsp is not set).
// Caller must defer mux.Close().
func setupLSP(ctx context.Context, workDir string, useLSP bool) *lsp.Multiplexer {
	if !useLSP {
		return nil
	}

	mux := lsp.NewMultiplexer(workDir)
	mux.ConnectAvailable(ctx)

	ids := mux.ConnectedServerIDs()
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, string(id))
	}
	if len(names) == 0 {
		names = append(names, "none")
	}
	_, _ = fmt.Fprintln(os.Stderr, fmt.Sprintf("LSP: connected to %s (%d servers)", strings.Join(names, ", "), len(ids)))
	return mux
}

func collectLSIFInputs(rootPath string) ([]string, error) {
	inputs, err := collectIndexInputs(rootPath, "", nil)
	if err != nil {
		return nil, err
	}

	filtered := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if lsifgen.IsSupportedExt(filepath.Ext(input)) {
			filtered = append(filtered, input)
		}
	}
	return filtered, nil
}

func loadSources(inputs []string) (map[string]string, error) {
	sources := make(map[string]string, len(inputs))
	for _, input := range inputs {
		content, err := os.ReadFile(input)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", input, err)
		}
		sources[input] = string(content)
	}
	return sources, nil
}

func writeLSIF(ctx context.Context, mux *lsp.Multiplexer, inputs []string, outputPath string) (*lsifgen.Stats, error) {
	sources, err := loadSources(inputs)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return nil, fmt.Errorf("create LSIF directory: %w", err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("create LSIF file: %w", err)
	}
	defer file.Close()

	generator := lsifgen.NewGenerator(mux)
	stats, err := generator.GenerateWithStats(ctx, sources, file)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func formatLSIFStats(stats *lsifgen.Stats) string {
	if stats == nil {
		return "LSIF: indexed 0 documents, 0 symbols, 0 references"
	}
	return fmt.Sprintf(
		"LSIF: indexed %d documents, %d symbols, %d references",
		stats.Documents,
		stats.Symbols,
		stats.References,
	)
}
