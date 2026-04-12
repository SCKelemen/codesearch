package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/SCKelemen/clix"
	"github.com/SCKelemen/codesearch"
	"github.com/SCKelemen/codesearch/hybrid"
	"github.com/SCKelemen/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	interactiveInputHeight  = 5
	interactiveStatusHeight = 1
	interactivePreviewLines = 16
)

type interactiveSearchTickMsg struct {
	id    int
	query string
}

type interactiveSearchResultsMsg struct {
	id      int
	query   string
	results []codesearch.Result
	err     error
}

type interactiveModel struct {
	ctx      context.Context
	engine   *codesearch.Engine
	indexDir string
	stats    indexStats

	input   *tui.TextInput
	status  *tui.StatusBar
	results *interactiveResultsList
	preview *tui.CodeBlock
	split   *tui.SplitPane

	query       string
	matches     []codesearch.Result
	selected    int
	showPreview bool
	searchSeq   int
	searchErr   error
	width       int
	height      int
	chosen      *codesearch.Result
}

type interactiveResultsList struct {
	width    int
	height   int
	query    string
	results  []codesearch.Result
	selected int
	loading  bool
	err      error
	focused  bool
}

func newInteractiveCommand() *clix.Command {
	cmd := clix.NewCommand("interactive")
	cmd.Short = "Launch the interactive search interface"
	cmd.Usage = "csx interactive [--index ./index]"

	var indexDir string
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "index", Short: "i", Usage: "Path to the local index directory"},
		Default:     defaultIndexDir,
		Value:       &indexDir,
	})

	cmd.Run = func(ctx *clix.Context) error {
		return runInteractive(ctx.App.Out, indexDir, defaultSearchLimit, hybrid.Hybrid)
	}
	return cmd
}

func runInteractive(out io.Writer, indexDir string, limit int, mode hybrid.SearchMode) error {
	engine, err := openEngine(indexDir)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer func() {
		_ = engine.Close()
	}()

	stats, err := collectIndexStats(context.Background(), engine, indexDir)
	if err != nil {
		return fmt.Errorf("collect status: %w", err)
	}

	program := tea.NewProgram(newInteractiveModel(context.Background(), indexDir, engine, stats, limit, mode), tea.WithAltScreen())
	finalModel, err := program.Run()
	if err != nil {
		return err
	}

	model, ok := finalModel.(*interactiveModel)
	if !ok || model.chosen == nil {
		return nil
	}
	if model.chosen.Line > 0 {
		_, err = fmt.Fprintf(out, "%s:%d\n", model.chosen.Path, model.chosen.Line)
		return err
	}
	_, err = fmt.Fprintln(out, model.chosen.Path)
	return err
}

func newInteractiveModel(ctx context.Context, indexDir string, engine *codesearch.Engine, stats indexStats, limit int, mode hybrid.SearchMode) *interactiveModel {
	input := tui.NewTextInput()
	input.Focus()

	status := tui.NewStatusBar()
	preview := placeholderPreview("Type to search the current index.")
	results := &interactiveResultsList{}
	split := tui.NewSplitPane(
		tui.WithRatio(0.55),
		tui.WithLeftComponent(results),
		tui.WithRightComponent(preview),
	)

	if limit <= 0 {
		limit = defaultSearchLimit
	}
	_ = mode

	model := &interactiveModel{
		ctx:         ctx,
		engine:      engine,
		indexDir:    indexDir,
		stats:       stats,
		input:       input,
		status:      status,
		results:     results,
		preview:     preview,
		split:       split,
		showPreview: true,
	}
	model.syncResults()
	model.refreshStatus()
	return model
}

func (m *interactiveModel) Init() tea.Cmd {
	return tea.Batch(m.input.Init(), m.status.Init(), m.split.Init())
}

func (m *interactiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch current := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = current.Width
		m.height = current.Height
		return m, m.updateLayout()
	case tea.KeyMsg:
		switch current.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		case "up", "k":
			m.moveSelection(-1)
			return m, nil
		case "down", "j":
			m.moveSelection(1)
			return m, nil
		case "tab":
			m.showPreview = !m.showPreview
			m.refreshStatus()
			return m, m.updateLayout()
		case "enter":
			if len(m.matches) == 0 {
				return m, nil
			}
			selected := m.matches[m.selected]
			m.chosen = &selected
			return m, tea.Quit
		}
	case interactiveSearchTickMsg:
		if current.id != m.searchSeq || current.query != m.query {
			return m, nil
		}
		m.results.loading = true
		m.refreshStatus()
		return m, m.search(current.id, current.query)
	case interactiveSearchResultsMsg:
		if current.id != m.searchSeq || current.query != m.query {
			return m, nil
		}
		m.results.loading = false
		m.searchErr = current.err
		if current.err != nil {
			m.matches = nil
			m.selected = 0
			m.syncResults()
			m.refreshStatus()
			return m, m.updateLayout()
		}
		m.matches = current.results
		if len(m.matches) == 0 {
			m.selected = 0
		} else if m.selected >= len(m.matches) {
			m.selected = len(m.matches) - 1
		}
		m.syncResults()
		m.refreshStatus()
		return m, m.updateLayout()
	}

	previousValue := strings.TrimSpace(m.input.Value())
	updatedInput, inputCmd := m.input.Update(msg)
	m.input = updatedInput.(*tui.TextInput)
	currentValue := strings.TrimSpace(m.input.Value())
	if previousValue == currentValue {
		return m, inputCmd
	}

	m.query = currentValue
	m.searchErr = nil
	if m.query == "" {
		m.matches = nil
		m.selected = 0
		m.results.loading = false
		m.syncResults()
		m.refreshStatus()
		return m, inputCmd
	}

	m.searchSeq++
	m.results.loading = true
	m.syncResults()
	m.refreshStatus()
	return m, tea.Batch(inputCmd, m.scheduleSearch(m.searchSeq, m.query))
}

func (m *interactiveModel) View() string {
	return strings.Join([]string{m.input.View(), m.mainView(), m.status.View()}, "")
}

func (m *interactiveModel) mainView() string {
	if m.contentHeight() <= 0 || m.width <= 0 {
		return ""
	}
	if m.showPreview {
		return m.split.View() + "\n"
	}
	return m.results.View() + "\n"
}

func (m *interactiveModel) scheduleSearch(id int, query string) tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return interactiveSearchTickMsg{id: id, query: query}
	})
}

func (m *interactiveModel) search(id int, query string) tea.Cmd {
	return func() tea.Msg {
		searchCtx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()

		results, err := m.engine.Search(
			searchCtx,
			query,
			codesearch.WithLimit(defaultSearchLimit),
			codesearch.WithMode(hybrid.Hybrid),
		)
		return interactiveSearchResultsMsg{id: id, query: query, results: results, err: err}
	}
}

func (m *interactiveModel) moveSelection(delta int) {
	if len(m.matches) == 0 {
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.matches) {
		m.selected = len(m.matches) - 1
	}
	m.syncResults()
	m.refreshStatus()
}

func (m *interactiveModel) syncResults() {
	m.results.query = m.query
	m.results.results = append(m.results.results[:0], m.matches...)
	m.results.selected = m.selected
	m.results.err = m.searchErr
	m.updatePreview()
}

func (m *interactiveModel) updatePreview() {
	if len(m.matches) == 0 {
		message := "Type a query to search the current index."
		if strings.TrimSpace(m.query) != "" {
			message = "No matches found for the current query."
		}
		if m.searchErr != nil {
			message = m.searchErr.Error()
		}
		m.preview = placeholderPreview(message)
		m.split.SetRightComponent(m.preview)
		return
	}
	m.preview = previewForResult(m.matches[m.selected])
	m.split.SetRightComponent(m.preview)
}

func (m *interactiveModel) refreshStatus() {
	message := fmt.Sprintf(
		"%s · %d files · %s · %d results · ↑/↓ move · Enter select · Tab preview · Esc quit",
		m.indexDir,
		m.stats.FileCount,
		humanBytes(m.stats.TotalBytes),
		len(m.matches),
	)
	if m.results.loading {
		message = fmt.Sprintf("Searching %q… · %s", m.query, message)
	}
	if m.searchErr != nil {
		message = fmt.Sprintf("Search error: %v · %s", m.searchErr, message)
	}
	m.status.SetMessage(message)
}

func (m *interactiveModel) updateLayout() tea.Cmd {
	if m.width <= 0 || m.height <= 0 {
		return nil
	}

	var cmds []tea.Cmd
	updatedInput, inputCmd := m.input.Update(tea.WindowSizeMsg{Width: m.width, Height: interactiveInputHeight})
	m.input = updatedInput.(*tui.TextInput)
	cmds = append(cmds, inputCmd)

	updatedStatus, statusCmd := m.status.Update(tea.WindowSizeMsg{Width: m.width, Height: interactiveStatusHeight})
	m.status = updatedStatus.(*tui.StatusBar)
	cmds = append(cmds, statusCmd)

	contentSize := tea.WindowSizeMsg{Width: m.width, Height: m.contentHeight()}
	updatedResults, resultsCmd := m.results.Update(contentSize)
	m.results = updatedResults.(*interactiveResultsList)
	cmds = append(cmds, resultsCmd)

	updatedSplit, splitCmd := m.split.Update(contentSize)
	m.split = updatedSplit.(*tui.SplitPane)
	cmds = append(cmds, splitCmd)
	return tea.Batch(cmds...)
}

func (m *interactiveModel) contentHeight() int {
	height := m.height - interactiveInputHeight - interactiveStatusHeight
	if height < 0 {
		return 0
	}
	return height
}

func previewForResult(result codesearch.Result) *tui.CodeBlock {
	lines := strings.Split(strings.ReplaceAll(result.Content, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	start := 0
	end := len(lines)
	if result.Line > 0 {
		start = result.Line - 5
		if start < 0 {
			start = 0
		}
		end = start + interactivePreviewLines
		if end > len(lines) {
			end = len(lines)
		}
		if end-start < interactivePreviewLines {
			start = end - interactivePreviewLines
			if start < 0 {
				start = 0
			}
		}
	}

	summary := fmt.Sprintf("%s · line %d · score %.3f", result.Path, result.Line, result.Score)
	if result.Line <= 0 {
		summary = fmt.Sprintf("%s · score %.3f", result.Path, result.Score)
	}

	return tui.NewCodeBlock(
		tui.WithCodeOperation("Preview"),
		tui.WithCodeFilename(result.Path),
		tui.WithCodeSummary(summary),
		tui.WithLanguage(languageForPath(result.Path)),
		tui.WithCodeLines(lines[start:end]),
		tui.WithStartLine(start+1),
		tui.WithExpanded(true),
	)
}

func placeholderPreview(message string) *tui.CodeBlock {
	return tui.NewCodeBlock(
		tui.WithCodeOperation("Preview"),
		tui.WithCodeFilename("selection"),
		tui.WithCodeSummary("Live preview updates as you move through results."),
		tui.WithCode(message),
		tui.WithExpanded(true),
	)
}

func (r *interactiveResultsList) Init() tea.Cmd {
	return nil
}

func (r *interactiveResultsList) Update(msg tea.Msg) (tui.Component, tea.Cmd) {
	switch current := msg.(type) {
	case tea.WindowSizeMsg:
		r.width = current.Width
		r.height = current.Height
	}
	return r, nil
}

func (r *interactiveResultsList) View() string {
	if r.width <= 0 || r.height <= 0 {
		return ""
	}

	lines := []string{resultHeaderStyle.Width(r.width).Render(r.header())}
	switch {
	case r.err != nil:
		lines = append(lines, resultEmptyStyle.Width(r.width).Render(r.err.Error()))
	case r.loading && len(r.results) == 0:
		lines = append(lines, resultEmptyStyle.Width(r.width).Render("Searching…"))
	case len(r.results) == 0:
		message := "Type to search the current index."
		if strings.TrimSpace(r.query) != "" {
			message = fmt.Sprintf("No results for %q.", r.query)
		}
		lines = append(lines, resultEmptyStyle.Width(r.width).Render(message))
	default:
		for _, index := range r.visibleIndexes() {
			lines = append(lines, r.renderResult(index, r.results[index]))
		}
	}

	for len(lines) < r.height {
		lines = append(lines, strings.Repeat(" ", r.width))
	}
	if len(lines) > r.height {
		lines = lines[:r.height]
	}
	return strings.Join(lines, "\n")
}

func (r *interactiveResultsList) Focus() {
	r.focused = true
}

func (r *interactiveResultsList) Blur() {
	r.focused = false
}

func (r *interactiveResultsList) Focused() bool {
	return r.focused
}

func (r *interactiveResultsList) header() string {
	if strings.TrimSpace(r.query) == "" {
		return "Results"
	}
	return fmt.Sprintf("Results for %q (%d)", r.query, len(r.results))
}

func (r *interactiveResultsList) visibleIndexes() []int {
	available := r.height - 1
	if available <= 0 {
		return nil
	}
	if len(r.results) <= available {
		indexes := make([]int, len(r.results))
		for i := range r.results {
			indexes[i] = i
		}
		return indexes
	}

	start := r.selected - available/2
	if start < 0 {
		start = 0
	}
	end := start + available
	if end > len(r.results) {
		end = len(r.results)
		start = end - available
	}

	indexes := make([]int, 0, end-start)
	for i := start; i < end; i++ {
		indexes = append(indexes, i)
	}
	return indexes
}

func (r *interactiveResultsList) renderResult(index int, result codesearch.Result) string {
	meta := fmt.Sprintf("L%-4d %.3f", result.Line, result.Score)
	pathWidth := r.width - lipgloss.Width(meta) - 4
	if pathWidth < 12 {
		pathWidth = 12
	}

	path := truncateMiddle(result.Path, pathWidth)
	line := lipgloss.JoinHorizontal(
		lipgloss.Top,
		resultPathStyle.Render(path),
		strings.Repeat(" ", max(1, r.width-lipgloss.Width(path)-lipgloss.Width(meta)-2)),
		resultMetaStyle.Render(meta),
	)

	prefix := "  "
	style := resultItemStyle
	if index == r.selected {
		prefix = "› "
		style = resultSelectedStyle
	}
	return style.Width(r.width).Render(prefix + line)
}

func truncateMiddle(value string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= maxWidth {
		return value
	}
	if maxWidth <= 1 {
		return "…"
	}

	runes := []rune(value)
	if len(runes) <= maxWidth {
		return value
	}
	keep := maxWidth - 1
	left := keep / 2
	right := keep - left
	if right > len(runes) {
		right = len(runes)
	}
	return string(runes[:left]) + "…" + string(runes[len(runes)-right:])
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

var (
	resultHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Padding(0, 1)
	resultItemStyle     = lipgloss.NewStyle().Padding(0, 1)
	resultSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("63")).
				Bold(true)
	resultPathStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	resultMetaStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	resultEmptyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244")).
				Padding(0, 1)
)
