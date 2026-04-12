package content

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/SCKelemen/codesearch/linguist"
)

var contentBenchLanguages int
var contentBenchBinaryCount int

func BenchmarkDetectLanguage(b *testing.B) {
	paths := benchmarkLanguagePaths(100)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		count := 0
		for _, path := range paths {
			if benchmarkDetectLanguage(path) != "" {
				count++
			}
		}
		contentBenchLanguages = count
	}
}

func BenchmarkIsBinary(b *testing.B) {
	inputs := [][]byte{
		[]byte("package main\n\nfunc HandleCheckoutRequest() string {\n\treturn \"checkout\"\n}\n"),
		[]byte("#!/usr/bin/env python3\n\nprint('billing export')\n"),
		append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 512)...),
		append([]byte("SQLite format 3\x00"), make([]byte, 1024)...),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		count := 0
		for _, input := range inputs {
			if benchmarkIsBinary(input) {
				count++
			}
		}
		contentBenchBinaryCount = count
	}
}

func benchmarkLanguagePaths(count int) []string {
	exts := []string{".go", ".ts", ".tsx", ".py", ".rs", ".java", ".sql", ".yaml", ".json", ".md"}
	paths := make([]string, count)
	for i := range paths {
		ext := exts[i%len(exts)]
		paths[i] = filepath.Join("workspace", "services", fmt.Sprintf("file_%03d%s", i, ext))
	}
	return paths
}

func benchmarkDetectLanguage(path string) string {
	language := linguist.LookupByExtension(filepath.Ext(path))
	if language == nil {
		return ""
	}
	return language.Name
}

func benchmarkIsBinary(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	sample := content
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	nonText := 0
	for _, b := range sample {
		if b == 0 {
			return true
		}
		if b < 0x09 || (b > 0x0d && b < 0x20) {
			nonText++
		}
	}
	return float64(nonText)/float64(len(sample)) > 0.2
}
