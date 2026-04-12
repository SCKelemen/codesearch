package lsifgen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/SCKelemen/codesearch/lsp"
)

// Generator walks source files, queries LSPs, and emits LSIF JSON lines.
type Generator struct {
	Mux     *lsp.Multiplexer
	nextID  atomic.Int64
	Version string
}

// Stats captures generation totals.
type Stats struct {
	Documents   int
	Symbols     int
	References  int
	HoverInfos  int
	Definitions int
	Languages   map[string]int
	Errors      int
}

type documentState struct {
	id       int
	path     string
	uri      string
	language string
	ranges   []int
	rangeSet map[int]struct{}
}

type rangeRef struct {
	rangeID int
}

// NewGenerator constructs an LSIF generator for the provided multiplexer.
func NewGenerator(mux *lsp.Multiplexer) *Generator {
	return &Generator{Mux: mux, Version: "0.4.3"}
}

// FormatStats formats generation statistics for display.
func FormatStats(stats *Stats) string {
	if stats == nil {
		return "no LSIF statistics collected"
	}
	languages := "none"
	if len(stats.Languages) > 0 {
		names := make([]string, 0, len(stats.Languages))
		for name := range stats.Languages {
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, name := range names {
			parts = append(parts, fmt.Sprintf("%s=%d", name, stats.Languages[name]))
		}
		languages = strings.Join(parts, ", ")
	}
	return fmt.Sprintf(
		"documents=%d symbols=%d references=%d hover infos=%d definitions=%d errors=%d languages=[%s]",
		stats.Documents,
		stats.Symbols,
		stats.References,
		stats.HoverInfos,
		stats.Definitions,
		stats.Errors,
		languages,
	)
}

// IsSupportedExt reports whether the extension is present in lsp.DefaultConfigs.
func IsSupportedExt(ext string) bool {
	normalized := strings.ToLower(ext)
	if normalized == "" {
		return false
	}
	if !strings.HasPrefix(normalized, ".") {
		normalized = "." + normalized
	}
	for _, cfg := range lsp.DefaultConfigs() {
		for _, cfgExt := range cfg.Extensions {
			if strings.EqualFold(cfgExt, normalized) {
				return true
			}
		}
	}
	return false
}

// Generate emits LSIF JSON lines and returns the number of emitted entries.
func (g *Generator) Generate(ctx context.Context, sources map[string]string, w io.Writer) (int, error) {
	return g.generate(ctx, sources, w, nil)
}

// GenerateWithStats emits LSIF JSON lines and collects statistics.
func (g *Generator) GenerateWithStats(ctx context.Context, sources map[string]string, w io.Writer) (*Stats, error) {
	stats := &Stats{Languages: make(map[string]int)}
	_, err := g.generate(ctx, sources, w, stats)
	return stats, err
}

func (g *Generator) generate(ctx context.Context, sources map[string]string, w io.Writer, stats *Stats) (int, error) {
	if g == nil {
		return 0, fmt.Errorf("nil generator")
	}
	if g.Mux == nil {
		return 0, fmt.Errorf("nil multiplexer")
	}
	if w == nil {
		return 0, fmt.Errorf("nil writer")
	}
	if stats != nil && stats.Languages == nil {
		stats.Languages = make(map[string]int)
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	count := 0

	g.nextID.Store(0)
	count++
	_ = g.emitVertex(enc, "metaData", map[string]any{
		"version":          g.version(),
		"positionEncoding": "utf-16",
		"toolInfo": map[string]any{
			"name":    "codesearch-lsifgen",
			"version": g.version(),
		},
	})
	projectID := g.emitVertex(enc, "project", map[string]any{
		"kind": "codesearch",
		"name": "codesearch",
	})
	count++

	documentStates := make(map[string]*documentState)
	documentOrder := make([]string, 0, len(sources))
	rangeCache := make(map[string]rangeRef)

	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		client := g.Mux.ClientForFile(path)
		if client == nil {
			if stats != nil {
				stats.Errors++
			}
			continue
		}

		uri := lsp.FileURI(path)
		docState, created := g.ensureDocument(enc, documentStates, path, uri, stats)
		if created {
			count++
			documentOrder = append(documentOrder, path)
		}

		content := sources[path]
		if err := client.OpenFile(ctx, uri, content); err != nil {
			if stats != nil {
				stats.Errors++
			}
			continue
		}

		symbols, err := client.DocumentSymbols(ctx, uri)
		if err != nil {
			if stats != nil {
				stats.Errors++
			}
			continue
		}

		for _, symbol := range flattenSymbols(symbols) {
			if err := ctx.Err(); err != nil {
				return count, err
			}
			rng := normalizeRange(symbol.Range, symbol.SelectionRange)
			rangeID, createdRange := g.ensureRange(enc, rangeCache, documentStates, path, uri, rng, map[string]any{
				"type": "definition",
				"text": symbol.Name,
				"kind": symbol.Kind,
			}, stats)
			if createdRange {
				count++
			}
			if stats != nil {
				stats.Symbols++
			}

			pos := symbolPosition(symbol)

			hover, err := client.Hover(ctx, uri, pos.Line, pos.Character)
			if err != nil {
				if stats != nil {
					stats.Errors++
				}
			} else if hover != nil && hover.Contents != "" {
				hoverID := g.emitVertex(enc, "hoverResult", map[string]any{
					"result": map[string]any{"contents": hover.Contents},
				})
				g.emitEdge(enc, "textDocument/hover", rangeID, hoverID, nil)
				count += 2
				if stats != nil {
					stats.HoverInfos++
				}
			}

			refs, err := client.References(ctx, uri, pos.Line, pos.Character, true)
			if err != nil {
				if stats != nil {
					stats.Errors++
				}
			} else if len(refs) > 0 {
				refResultID := g.emitVertex(enc, "referenceResult", map[string]any{})
				g.emitEdge(enc, "textDocument/references", rangeID, refResultID, nil)
				count += 2
				refRangeIDs := make([]int, 0, len(refs))
				for _, loc := range refs {
					refRangeID, createdRefRange := g.ensureRangeForLocation(enc, rangeCache, documentStates, loc, map[string]any{"type": "reference"}, stats)
					if createdRefRange {
						count++
					}
					refRangeIDs = append(refRangeIDs, refRangeID)
				}
				if len(refRangeIDs) > 0 {
					g.emitEdge(enc, "item", refResultID, 0, refRangeIDs)
					count++
				}
				if stats != nil {
					stats.References += len(refs)
				}
			}

			defs, err := client.Definition(ctx, uri, pos.Line, pos.Character)
			if err != nil {
				if stats != nil {
					stats.Errors++
				}
			} else if len(defs) > 0 {
				defResultID := g.emitVertex(enc, "definitionResult", map[string]any{})
				g.emitEdge(enc, "textDocument/definition", rangeID, defResultID, nil)
				count += 2
				defRangeIDs := make([]int, 0, len(defs))
				for _, loc := range defs {
					defRangeID, createdDefRange := g.ensureRangeForLocation(enc, rangeCache, documentStates, loc, map[string]any{"type": "definition"}, stats)
					if createdDefRange {
						count++
					}
					defRangeIDs = append(defRangeIDs, defRangeID)
				}
				if len(defRangeIDs) > 0 {
					g.emitEdge(enc, "item", defResultID, 0, defRangeIDs)
					count++
				}
				if stats != nil {
					stats.Definitions += len(defs)
				}
			}

			_ = docState
		}
	}

	projectDocIDs := make([]int, 0, len(documentOrder))
	for _, path := range documentOrder {
		docState := documentStates[path]
		projectDocIDs = append(projectDocIDs, docState.id)
		if len(docState.ranges) > 0 {
			g.emitEdge(enc, "contains", docState.id, 0, docState.ranges)
			count++
		}
	}
	if len(projectDocIDs) > 0 {
		g.emitEdge(enc, "contains", projectID, 0, projectDocIDs)
		count++
	}

	return count, nil
}

func (g *Generator) ensureDocument(enc *json.Encoder, docs map[string]*documentState, path, uri string, stats *Stats) (*documentState, bool) {
	if doc, ok := docs[path]; ok {
		return doc, false
	}
	language := languageForPath(path)
	id := g.emitVertex(enc, "document", map[string]any{
		"uri":        uri,
		"languageId": language,
		"path":       path,
		"version":    1,
	})
	doc := &documentState{
		id:       id,
		path:     path,
		uri:      uri,
		language: language,
		rangeSet: make(map[int]struct{}),
	}
	docs[path] = doc
	if stats != nil {
		stats.Documents++
		stats.Languages[language]++
	}
	return doc, true
}

func (g *Generator) ensureRange(
	enc *json.Encoder,
	rangeCache map[string]rangeRef,
	docs map[string]*documentState,
	path, uri string,
	rng lsp.Range,
	tag map[string]any,
	stats *Stats,
) (int, bool) {
	docState, _ := g.ensureDocument(enc, docs, path, uri, stats)
	key := rangeKey(path, rng, tag)
	if ref, ok := rangeCache[key]; ok {
		return ref.rangeID, false
	}
	id := g.emitVertex(enc, "range", map[string]any{
		"start": map[string]any{"line": rng.Start.Line, "character": rng.Start.Character},
		"end":   map[string]any{"line": rng.End.Line, "character": rng.End.Character},
		"tag":   tag,
	})
	docState.addRange(id)
	rangeCache[key] = rangeRef{rangeID: id}
	return id, true
}

func (g *Generator) ensureRangeForLocation(
	enc *json.Encoder,
	rangeCache map[string]rangeRef,
	docs map[string]*documentState,
	loc lsp.Location,
	tag map[string]any,
	stats *Stats,
) (int, bool) {
	path := lsp.URIToPath(loc.URI)
	if path == "" {
		path = loc.URI
	}
	uri := loc.URI
	if uri == "" {
		uri = lsp.FileURI(path)
	}
	return g.ensureRange(enc, rangeCache, docs, path, uri, loc.Range, tag, stats)
}

func (d *documentState) addRange(id int) {
	if _, ok := d.rangeSet[id]; ok {
		return
	}
	d.rangeSet[id] = struct{}{}
	d.ranges = append(d.ranges, id)
}

func flattenSymbols(symbols []lsp.Symbol) []lsp.Symbol {
	out := make([]lsp.Symbol, 0, len(symbols))
	var walk func([]lsp.Symbol)
	walk = func(items []lsp.Symbol) {
		for _, symbol := range items {
			out = append(out, symbol)
			if len(symbol.Children) > 0 {
				walk(symbol.Children)
			}
		}
	}
	walk(symbols)
	return out
}

func normalizeRange(primary, fallback lsp.Range) lsp.Range {
	if primary != (lsp.Range{}) {
		return primary
	}
	return fallback
}

func symbolPosition(symbol lsp.Symbol) lsp.Position {
	if symbol.SelectionRange != (lsp.Range{}) {
		return symbol.SelectionRange.Start
	}
	return symbol.Range.Start
}

func rangeKey(path string, rng lsp.Range, tag map[string]any) string {
	return fmt.Sprintf(
		"%s:%d:%d:%d:%d:%v",
		path,
		rng.Start.Line,
		rng.Start.Character,
		rng.End.Line,
		rng.End.Character,
		tag,
	)
}

func languageForPath(path string) string {
	if lang := lsp.LangIDForExt(filepath.Ext(path)); lang != "" {
		return lang
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "" {
		return "plaintext"
	}
	return ext
}

func (g *Generator) version() string {
	if g.Version != "" {
		return g.Version
	}
	return "0.4.3"
}

func (g *Generator) emitVertex(enc *json.Encoder, label string, data map[string]any) int {
	id := int(g.nextID.Add(1))
	entry := map[string]any{"id": id, "type": "vertex", "label": label}
	for key, value := range data {
		entry[key] = value
	}
	_ = enc.Encode(entry)
	return id
}

func (g *Generator) emitEdge(enc *json.Encoder, label string, outV, inV int, inVs []int) {
	id := int(g.nextID.Add(1))
	entry := map[string]any{"id": id, "type": "edge", "label": label, "outV": outV}
	if len(inVs) > 0 {
		entry["inVs"] = inVs
	} else {
		entry["inV"] = inV
	}
	_ = enc.Encode(entry)
}
