package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/SCKelemen/clix"
	"github.com/SCKelemen/codesearch"
	codesearchv1 "github.com/SCKelemen/codesearch/proto/codesearchv1"
)

func newSearchCommand() *clix.Command {
	cmd := clix.NewCommand("search")
	cmd.Short = "Search a local or remote index"
	cmd.Usage = "csx search [query] [--index ./index] [--limit 20] [--mode hybrid|lexical|semantic] [--json] [--remote <addr>]"
	cmd.Arguments = []*clix.Argument{{
		Name:     "query",
		Prompt:   "Search query",
		Required: false,
		Validate: func(value string) error {
			return requireQuery(value)
		},
	}}

	var indexDir string
	var remote string
	var mode string
	var limit int
	var jsonOutput bool

	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "index", Short: "i", Usage: "Path to the local index directory"},
		Default:     defaultIndexDir,
		Value:       &indexDir,
	})
	cmd.Flags.IntVar(clix.IntVarOptions{
		FlagOptions: clix.FlagOptions{Name: "limit", Short: "n", Usage: "Maximum number of results to return"},
		Default:     "20",
		Value:       &limit,
	})
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "mode", Short: "m", Usage: "Search mode: hybrid, lexical, or semantic"},
		Default:     "hybrid",
		Value:       &mode,
	})
	cmd.Flags.BoolVar(clix.BoolVarOptions{
		FlagOptions: clix.FlagOptions{Name: "json", Usage: "Emit machine-readable JSON"},
		Value:       &jsonOutput,
	})
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "remote", Short: "r", Usage: "Remote server address, for example 127.0.0.1:8080"},
		Value:       &remote,
	})

	cmd.Run = func(ctx *clix.Context) error {
		if len(ctx.Args) == 0 {
			if jsonOutput {
				return fmt.Errorf("interactive mode does not support --json")
			}
			if strings.TrimSpace(remote) != "" {
				return fmt.Errorf("interactive mode does not support --remote")
			}
			searchMode, err := normalizeMode(mode)
			if err != nil {
				return err
			}
			return runInteractive(ctx.App.Out, indexDir, limit, searchMode)
		}

		response, err := runSearch(ctx, indexDir, remote, searchRequest{Query: ctx.Args[0], Limit: limit, Mode: mode})
		if err != nil {
			return err
		}
		if jsonOutput {
			return renderSearchJSON(ctx.App.Out, response)
		}
		return renderSearchText(ctx.App.Out, response)
	}
	return cmd
}

func runSearch(ctx *clix.Context, indexDir, remote string, request searchRequest) (searchResponse, error) {
	mode, err := normalizeMode(request.Mode)
	if err != nil {
		return searchResponse{}, err
	}
	limit := request.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if strings.TrimSpace(remote) != "" {
		remoteRequest := searchRequest{Query: request.Query, Limit: limit, Mode: modeLabel(mode)}
		response, err := httpConnectSearch(ctx, http.DefaultClient, remote, remoteRequest)
		if err == nil {
			return response, nil
		}
		if !shouldFallbackConnect(err) {
			return searchResponse{}, err
		}
		return httpSearch(ctx, http.DefaultClient, remote, remoteRequest)
	}
	engine, err := openEngine(indexDir)
	if err != nil {
		return searchResponse{}, fmt.Errorf("open index: %w", err)
	}
	defer func() {
		_ = engine.Close()
	}()
	results, err := engine.Search(ctx, request.Query, codesearch.WithLimit(limit), codesearch.WithMode(mode))
	if err != nil {
		return searchResponse{}, err
	}
	return buildSearchResponse(request.Query, limit, mode, "local", results), nil
}

func renderSearchJSON(out io.Writer, response searchResponse) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(response)
}

func renderSearchText(out io.Writer, response searchResponse) error {
	ui := newCLIUI(out)
	ui.section("Search results")
	ui.kv("query", fmt.Sprintf("%q", response.Query))
	ui.kv("mode", response.Mode)
	if response.Source != "" {
		ui.kv("source", response.Source)
	}
	ui.kv("results", fmt.Sprintf("%d", len(response.Results)))
	if len(response.Results) == 0 {
		ui.warnf("no matches found")
		return nil
	}

	results := append([]searchResult(nil), response.Results...)
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Path != results[j].Path {
			return results[i].Path < results[j].Path
		}
		return results[i].Line < results[j].Line
	})
	for i, result := range results {
		location := ""
		if result.Line > 0 {
			location = ui.label(fmt.Sprintf(":%d", result.Line))
		}
		ui.println(fmt.Sprintf("%2d. %s%s %s", i+1, ui.path(result.Path, 72), location, ui.label(fmt.Sprintf("%.3f", result.Score))))
		if result.Snippet != "" {
			ui.println("    " + highlightSnippet(ui, result.Snippet, result.Matches))
		}
	}
	return nil
}

func highlightSnippet(ui *cliUI, snippet string, matches []matchRange) string {
	text := ui.snippet(snippet, 110)
	if len(matches) == 0 {
		return text
	}
	var builder strings.Builder
	position := 0
	for _, match := range matches {
		start := clamp(match.Start, 0, len(text))
		end := clamp(match.End, 0, len(text))
		if start < position || start >= end {
			continue
		}
		builder.WriteString(text[position:start])
		builder.WriteString(ui.paint(ui.accent, text[start:end]))
		position = end
	}
	builder.WriteString(text[position:])
	return builder.String()
}

func clamp(value, lower, upper int) int {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}

type connectFallbackError struct {
	err error
}

func (e *connectFallbackError) Error() string {
	return e.err.Error()
}

func (e *connectFallbackError) Unwrap() error {
	return e.err
}

func shouldFallbackConnect(err error) bool {
	var fallback *connectFallbackError
	return errors.As(err, &fallback)
}

func httpConnectSearch(ctx context.Context, client *http.Client, address string, request searchRequest) (searchResponse, error) {
	mode, err := normalizeMode(request.Mode)
	if err != nil {
		return searchResponse{}, err
	}
	limit := request.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	endpoint := normalizeAddress(address) + codesearchv1.SearchProcedurePath
	body, err := json.Marshal(codesearchv1.SearchRequest{
		Query: request.Query,
		Limit: int32(limit),
		Mode:  modeLabel(mode),
	})
	if err != nil {
		return searchResponse{}, fmt.Errorf("marshal connect request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return searchResponse{}, fmt.Errorf("build connect request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Connect-Protocol-Version", "1")
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := client.Do(httpRequest)
	if err != nil {
		return searchResponse{}, fmt.Errorf("remote connect search failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusMethodNotAllowed || response.StatusCode == http.StatusUnsupportedMediaType {
		return searchResponse{}, &connectFallbackError{err: fmt.Errorf("connect endpoint unavailable: %s", response.Status)}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := readRemoteError(response.Body)
		if message == "" {
			message = response.Status
		}
		return searchResponse{}, fmt.Errorf("remote connect search failed: %s", message)
	}

	var payload codesearchv1.SearchResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return searchResponse{}, &connectFallbackError{err: fmt.Errorf("decode connect response: %w", err)}
	}
	return searchResponseFromProto(payload), nil
}

func searchResponseFromProto(response codesearchv1.SearchResponse) searchResponse {
	converted := searchResponse{
		Query:   response.Query,
		Limit:   int(response.Limit),
		Mode:    response.Mode,
		Source:  "remote",
		Results: make([]searchResult, 0, len(response.Results)),
	}
	for _, result := range response.Results {
		entry := searchResult{
			Path:    result.Path,
			Line:    int(result.Line),
			Score:   result.Score,
			Snippet: result.Snippet,
		}
		if len(result.Matches) != 0 {
			entry.Matches = make([]matchRange, 0, len(result.Matches))
			for _, match := range result.Matches {
				entry.Matches = append(entry.Matches, matchRange{Start: int(match.Start), End: int(match.End)})
			}
		}
		converted.Results = append(converted.Results, entry)
	}
	return converted
}

func readRemoteError(body io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(body, 4096))
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return ""
	}

	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &payload) == nil {
		if payload.Message != "" {
			return payload.Message
		}
		if payload.Error != "" {
			return payload.Error
		}
	}
	return trimmed
}
