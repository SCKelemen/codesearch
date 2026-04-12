package codesearchv1

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/SCKelemen/codesearch"
	codesearchpb "github.com/SCKelemen/codesearch/gen/codesearch/v1"
	"github.com/SCKelemen/codesearch/gen/codesearch/v1/codesearchv1connect"
	"github.com/SCKelemen/codesearch/store"
	"github.com/SCKelemen/codesearch/structural"
)

var _ codesearchv1connect.CodeSearchServiceHandler = (*Service)(nil)

// Service implements the generated Connect code search service.
type Service struct {
	engine *codesearch.Engine
}

// NewService constructs a Connect-compatible code search service.
func NewService(engine *codesearch.Engine) *Service {
	return &Service{engine: engine}
}

func (s *Service) Search(ctx context.Context, req *connect.Request[codesearchpb.SearchRequest]) (*connect.Response[codesearchpb.SearchResponse], error) {
	request := req.Msg
	if strings.TrimSpace(request.GetQuery()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("query must not be empty"))
	}

	mode, err := normalizeMode(request.GetMode())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	limit := int(request.GetLimit())
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	searchOptions := []codesearch.SearchOption{
		codesearch.WithLimit(limit),
		codesearch.WithMode(mode),
	}
	if filter := strings.TrimSpace(request.GetFilter()); filter != "" {
		searchOptions = append(searchOptions, codesearch.WithFilter(filter))
	}

	results, err := s.engine.Search(ctx, request.GetQuery(), searchOptions...)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	response := &codesearchpb.SearchResponse{
		Query:   request.GetQuery(),
		Limit:   int32(limit),
		Mode:    modeLabel(mode),
		Results: make([]*codesearchpb.SearchResult, 0, len(results)),
	}
	for _, result := range results {
		response.Results = append(response.Results, searchResultToProto(result))
	}
	return connect.NewResponse(response), nil
}

func (s *Service) IndexStatus(ctx context.Context, _ *connect.Request[codesearchpb.IndexStatusRequest]) (*connect.Response[codesearchpb.IndexStatusResponse], error) {
	response, err := collectIndexStatus(ctx, s.engine)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(response), nil
}

func (s *Service) SearchSymbols(ctx context.Context, req *connect.Request[codesearchpb.SearchSymbolsRequest]) (*connect.Response[codesearchpb.SearchSymbolsResponse], error) {
	query, limit, err := symbolQueryFromProto(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	results, err := s.engine.SearchSymbols(ctx, query)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	response := &codesearchpb.SearchSymbolsResponse{Results: make([]*codesearchpb.SymbolResult, 0, len(results))}
	for _, result := range results {
		response.Results = append(response.Results, symbolResultToProto(result))
	}
	return connect.NewResponse(response), nil
}

func searchResultToProto(result codesearch.Result) *codesearchpb.SearchResult {
	response := &codesearchpb.SearchResult{
		Path:    result.Path,
		Line:    int32(result.Line),
		Score:   result.Score,
		Snippet: result.Snippet,
	}
	if len(result.Matches) == 0 {
		return response
	}

	response.Matches = make([]*codesearchpb.MatchRange, 0, len(result.Matches))
	for _, match := range result.Matches {
		response.Matches = append(response.Matches, &codesearchpb.MatchRange{
			Start: int32(match.Start),
			End:   int32(match.End),
		})
	}
	return response
}

func collectIndexStatus(ctx context.Context, engine *codesearch.Engine) (*codesearchpb.IndexStatusResponse, error) {
	response := &codesearchpb.IndexStatusResponse{Languages: make(map[string]int32)}
	cursor := ""
	for {
		documents, next, err := engine.Documents.List(ctx, store.WithLimit(512), store.WithCursor(cursor))
		if err != nil {
			return nil, err
		}
		for _, document := range documents {
			response.FileCount++
			response.TotalBytes += document.Size
			response.IndexBytes += document.Size
			language := document.Language
			if language == "" {
				language = languageForPath(document.Path)
			}
			if language == "" {
				language = "unknown"
			}
			response.Languages[language]++
		}
		if next == "" {
			break
		}
		cursor = next
	}

	vectorCursor := ""
	for {
		vectors, next, err := engine.Vectors.List(ctx, store.WithLimit(512), store.WithCursor(vectorCursor))
		if err != nil {
			return nil, err
		}
		response.EmbeddingCount += int32(len(vectors))
		if next == "" {
			break
		}
		vectorCursor = next
	}

	return response, nil
}

func symbolQueryFromProto(request *codesearchpb.SearchSymbolsRequest) (structural.SymbolQuery, int, error) {
	if request == nil {
		return structural.SymbolQuery{}, 0, nil
	}

	kind, err := parseSymbolKind(request.GetKind())
	if err != nil {
		return structural.SymbolQuery{}, 0, err
	}

	query := structural.SymbolQuery{
		Name:      request.GetName(),
		Kind:      kind,
		Language:  request.GetLanguage(),
		Container: request.GetContainer(),
		Path:      request.GetPath(),
	}
	return query, int(request.GetLimit()), nil
}

func symbolResultToProto(result structural.Symbol) *codesearchpb.SymbolResult {
	return &codesearchpb.SymbolResult{
		Name:      result.Name,
		Kind:      formatSymbolKind(result.Kind),
		Language:  result.Language,
		Path:      result.Path,
		Container: result.Container,
		Exported:  result.Exported,
		Range: &codesearchpb.SourceRange{
			StartLine:   int32(result.Range.StartLine),
			StartColumn: int32(result.Range.StartColumn),
			EndLine:     int32(result.Range.EndLine),
			EndColumn:   int32(result.Range.EndColumn),
		},
	}
}

func parseSymbolKind(raw string) (structural.SymbolKind, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "unknown":
		return structural.SymbolKindUnknown, nil
	case "package":
		return structural.SymbolKindPackage, nil
	case "module":
		return structural.SymbolKindModule, nil
	case "class":
		return structural.SymbolKindClass, nil
	case "interface":
		return structural.SymbolKindInterface, nil
	case "struct":
		return structural.SymbolKindStruct, nil
	case "enum":
		return structural.SymbolKindEnum, nil
	case "trait":
		return structural.SymbolKindTrait, nil
	case "type":
		return structural.SymbolKindType, nil
	case "function":
		return structural.SymbolKindFunction, nil
	case "method":
		return structural.SymbolKindMethod, nil
	case "field":
		return structural.SymbolKindField, nil
	case "variable":
		return structural.SymbolKindVariable, nil
	case "constant":
		return structural.SymbolKindConstant, nil
	default:
		return structural.SymbolKindUnknown, fmt.Errorf("unknown symbol kind %q", raw)
	}
}

func formatSymbolKind(kind structural.SymbolKind) string {
	switch kind {
	case structural.SymbolKindPackage:
		return "package"
	case structural.SymbolKindModule:
		return "module"
	case structural.SymbolKindClass:
		return "class"
	case structural.SymbolKindInterface:
		return "interface"
	case structural.SymbolKindStruct:
		return "struct"
	case structural.SymbolKindEnum:
		return "enum"
	case structural.SymbolKindTrait:
		return "trait"
	case structural.SymbolKindType:
		return "type"
	case structural.SymbolKindFunction:
		return "function"
	case structural.SymbolKindMethod:
		return "method"
	case structural.SymbolKindField:
		return "field"
	case structural.SymbolKindVariable:
		return "variable"
	case structural.SymbolKindConstant:
		return "constant"
	default:
		return "unknown"
	}
}
