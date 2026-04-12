package main

import (
	"fmt"
	"io"
	"time"

	"github.com/SCKelemen/clix"
)

func newStatusCommand() *clix.Command {
	cmd := clix.NewCommand("status")
	cmd.Short = "Show local index statistics"
	cmd.Usage = "csx status [--index ./index]"

	var indexDir string
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "index", Short: "i", Usage: "Path to the local index directory"},
		Default:     defaultIndexDir,
		Value:       &indexDir,
	})

	cmd.Run = func(ctx *clix.Context) error {
		engine, err := openEngine(indexDir)
		if err != nil {
			return fmt.Errorf("open index: %w", err)
		}
		defer func() {
			_ = engine.Close()
		}()
		stats, err := collectIndexStats(ctx, engine, indexDir)
		if err != nil {
			return fmt.Errorf("collect status: %w", err)
		}
		return renderStatus(ctx.App.Out, indexDir, stats)
	}
	return cmd
}

func renderStatus(out io.Writer, indexDir string, stats indexStats) error {
	ui := newCLIUI(out)
	ui.section("Index status")
	ui.kv("index", indexDir)
	ui.kv("files", fmt.Sprintf("%d", stats.FileCount))
	ui.kv("content", humanBytes(stats.TotalBytes))
	ui.kv("disk", humanBytes(stats.IndexBytes))
	ui.kv("embeddings", fmt.Sprintf("%d", stats.EmbeddingCount))
	if stats.LastModified.IsZero() {
		ui.kv("updated", "never")
	} else {
		ui.kv("updated", stats.LastModified.Format(time.RFC3339))
	}
	if stats.FileCount == 0 {
		ui.warnf("index is empty")
		return nil
	}
	ui.println("")
	ui.section("Languages")
	for _, line := range sortLanguageCounts(stats.Languages) {
		ui.println("  " + line)
	}
	return nil
}
