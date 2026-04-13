package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/SCKelemen/clix"
	"github.com/SCKelemen/codesearch/lsp/lsifgen"
)

func newLSIFCommand() *clix.Command {
	cmd := clix.NewCommand("lsif")
	cmd.Short = "Generate LSIF output for a file or directory"
	cmd.Usage = "csx lsif <path> [--output <file>]"
	cmd.Arguments = []*clix.Argument{{
		Name:     "path",
		Prompt:   "Path to generate LSIF for",
		Required: true,
	}}

	var outputPath string
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "output", Short: "o", Usage: "Write LSIF JSON lines to a file instead of stdout"},
		Value:       &outputPath,
	})

	cmd.Run = func(ctx *clix.Context) error {
		resolvedRoot, err := filepath.Abs(ctx.Args[0])
		if err != nil {
			return fmt.Errorf("resolve input path: %w", err)
		}

		workDir := resolvedRoot
		info, err := os.Stat(resolvedRoot)
		if err != nil {
			return fmt.Errorf("stat %s: %w", resolvedRoot, err)
		}
		if !info.IsDir() {
			workDir = filepath.Dir(resolvedRoot)
		}

		mux := setupLSP(ctx, workDir, true)
		defer func() {
			if mux != nil {
				_ = mux.Close()
			}
		}()

		inputs, err := collectLSIFInputs(resolvedRoot)
		if err != nil {
			return err
		}
		if len(inputs) == 0 {
			return fmt.Errorf("no LSIF-supported files found under %s", resolvedRoot)
		}

		sources, err := loadSources(inputs)
		if err != nil {
			return err
		}

		writer := ctx.App.Out
		var file *os.File
		if outputPath != "" {
			resolvedOutput, err := filepath.Abs(outputPath)
			if err != nil {
				return fmt.Errorf("resolve output path: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(resolvedOutput), 0o755); err != nil {
				return fmt.Errorf("create output directory: %w", err)
			}
			file, err = os.Create(resolvedOutput)
			if err != nil {
				return fmt.Errorf("create output file: %w", err)
			}
			defer file.Close()
			writer = file
		}

		generator := lsifgen.NewGenerator(mux)
		stats, err := generator.GenerateWithStats(ctx, sources, writer)
		if err != nil {
			return fmt.Errorf("generate LSIF: %w", err)
		}
		_, _ = fmt.Fprintln(ctx.App.Err, formatLSIFStats(stats))
		return nil
	}

	return cmd
}
