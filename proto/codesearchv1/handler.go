package codesearchv1

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/SCKelemen/codesearch"
	"github.com/SCKelemen/codesearch/gen/codesearch/v1/codesearchv1connect"
	"github.com/SCKelemen/codesearch/hybrid"
	"github.com/SCKelemen/codesearch/linguist"
)

const (
	SearchProcedurePath        = codesearchv1connect.CodeSearchServiceSearchProcedure
	IndexStatusProcedurePath   = codesearchv1connect.CodeSearchServiceIndexStatusProcedure
	SearchSymbolsProcedurePath = codesearchv1connect.CodeSearchServiceSearchSymbolsProcedure
	defaultSearchLimit         = 20
)

// CodeSearchHandler is a deprecated compatibility wrapper around the generated Connect handler.
//
// Deprecated: use Service with codesearchv1connect.NewCodeSearchServiceHandler.
type CodeSearchHandler struct {
	handler http.Handler
}

// NewCodeSearchHandler constructs a deprecated compatibility wrapper around the generated Connect handler.
//
// Deprecated: use NewService and codesearchv1connect.NewCodeSearchServiceHandler.
func NewCodeSearchHandler(engine *codesearch.Engine) *CodeSearchHandler {
	_, handler := codesearchv1connect.NewCodeSearchServiceHandler(NewService(engine))
	return &CodeSearchHandler{handler: handler}
}

func (h *CodeSearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

func normalizeMode(raw string) (hybrid.SearchMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "hybrid":
		return hybrid.Hybrid, nil
	case "lexical", "lexical_only":
		return hybrid.LexicalOnly, nil
	case "semantic", "semantic_only":
		return hybrid.SemanticOnly, nil
	default:
		return "", fmt.Errorf("unknown search mode %q", raw)
	}
}

func modeLabel(mode hybrid.SearchMode) string {
	switch mode {
	case hybrid.LexicalOnly:
		return "lexical"
	case hybrid.SemanticOnly:
		return "semantic"
	default:
		return "hybrid"
	}
}

func languageForPath(path string) string {
	language := linguist.LookupByExtension(filepath.Ext(path))
	if language == nil {
		return ""
	}
	return language.Name
}
