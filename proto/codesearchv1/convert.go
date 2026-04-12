package codesearchv1

import codesearchpb "github.com/SCKelemen/codesearch/gen/codesearch/v1"

func SearchRequestFromProto(request *codesearchpb.SearchRequest) SearchRequest {
	if request == nil {
		return SearchRequest{}
	}
	return SearchRequest{
		Query:  request.GetQuery(),
		Limit:  request.GetLimit(),
		Mode:   request.GetMode(),
		Filter: request.GetFilter(),
	}
}

func (request SearchRequest) ToProto() *codesearchpb.SearchRequest {
	return &codesearchpb.SearchRequest{
		Query:  request.Query,
		Limit:  request.Limit,
		Mode:   request.Mode,
		Filter: request.Filter,
	}
}

func SearchResponseFromProto(response *codesearchpb.SearchResponse) SearchResponse {
	if response == nil {
		return SearchResponse{}
	}
	results := make([]SearchResult, 0, len(response.GetResults()))
	for _, result := range response.GetResults() {
		results = append(results, SearchResultFromProto(result))
	}
	return SearchResponse{
		Query:   response.GetQuery(),
		Limit:   response.GetLimit(),
		Mode:    response.GetMode(),
		Results: results,
	}
}

func (response SearchResponse) ToProto() *codesearchpb.SearchResponse {
	results := make([]*codesearchpb.SearchResult, 0, len(response.Results))
	for _, result := range response.Results {
		results = append(results, result.ToProto())
	}
	return &codesearchpb.SearchResponse{
		Query:   response.Query,
		Limit:   response.Limit,
		Mode:    response.Mode,
		Results: results,
	}
}

func SearchResultFromProto(result *codesearchpb.SearchResult) SearchResult {
	if result == nil {
		return SearchResult{}
	}
	matches := make([]MatchRange, 0, len(result.GetMatches()))
	for _, match := range result.GetMatches() {
		matches = append(matches, MatchRangeFromProto(match))
	}
	return SearchResult{
		Path:    result.GetPath(),
		Line:    result.GetLine(),
		Score:   result.GetScore(),
		Snippet: result.GetSnippet(),
		Matches: matches,
	}
}

func (result SearchResult) ToProto() *codesearchpb.SearchResult {
	matches := make([]*codesearchpb.MatchRange, 0, len(result.Matches))
	for _, match := range result.Matches {
		matches = append(matches, match.ToProto())
	}
	return &codesearchpb.SearchResult{
		Path:    result.Path,
		Line:    result.Line,
		Score:   result.Score,
		Snippet: result.Snippet,
		Matches: matches,
	}
}

func MatchRangeFromProto(match *codesearchpb.MatchRange) MatchRange {
	if match == nil {
		return MatchRange{}
	}
	return MatchRange{
		Start: match.GetStart(),
		End:   match.GetEnd(),
	}
}

func (match MatchRange) ToProto() *codesearchpb.MatchRange {
	return &codesearchpb.MatchRange{
		Start: match.Start,
		End:   match.End,
	}
}

func SearchSymbolsRequestFromProto(request *codesearchpb.SearchSymbolsRequest) SearchSymbolsRequest {
	if request == nil {
		return SearchSymbolsRequest{}
	}
	return SearchSymbolsRequest{
		Name:      request.GetName(),
		Kind:      request.GetKind(),
		Language:  request.GetLanguage(),
		Container: request.GetContainer(),
		Path:      request.GetPath(),
		Limit:     request.GetLimit(),
	}
}

func (request SearchSymbolsRequest) ToProto() *codesearchpb.SearchSymbolsRequest {
	return &codesearchpb.SearchSymbolsRequest{
		Name:      request.Name,
		Kind:      request.Kind,
		Language:  request.Language,
		Container: request.Container,
		Path:      request.Path,
		Limit:     request.Limit,
	}
}

func SearchSymbolsResponseFromProto(response *codesearchpb.SearchSymbolsResponse) SearchSymbolsResponse {
	if response == nil {
		return SearchSymbolsResponse{}
	}
	results := make([]SymbolResult, 0, len(response.GetResults()))
	for _, result := range response.GetResults() {
		results = append(results, SymbolResultFromProto(result))
	}
	return SearchSymbolsResponse{Results: results}
}

func (response SearchSymbolsResponse) ToProto() *codesearchpb.SearchSymbolsResponse {
	results := make([]*codesearchpb.SymbolResult, 0, len(response.Results))
	for _, result := range response.Results {
		results = append(results, result.ToProto())
	}
	return &codesearchpb.SearchSymbolsResponse{Results: results}
}

func SymbolResultFromProto(result *codesearchpb.SymbolResult) SymbolResult {
	if result == nil {
		return SymbolResult{}
	}
	return SymbolResult{
		Name:      result.GetName(),
		Kind:      result.GetKind(),
		Language:  result.GetLanguage(),
		Path:      result.GetPath(),
		Container: result.GetContainer(),
		Exported:  result.GetExported(),
		Range:     SourceRangeFromProto(result.GetRange()),
	}
}

func (result SymbolResult) ToProto() *codesearchpb.SymbolResult {
	return &codesearchpb.SymbolResult{
		Name:      result.Name,
		Kind:      result.Kind,
		Language:  result.Language,
		Path:      result.Path,
		Container: result.Container,
		Exported:  result.Exported,
		Range:     result.Range.ToProto(),
	}
}

func SourceRangeFromProto(sourceRange *codesearchpb.SourceRange) SourceRange {
	if sourceRange == nil {
		return SourceRange{}
	}
	return SourceRange{
		StartLine:   sourceRange.GetStartLine(),
		StartColumn: sourceRange.GetStartColumn(),
		EndLine:     sourceRange.GetEndLine(),
		EndColumn:   sourceRange.GetEndColumn(),
	}
}

func (sourceRange SourceRange) ToProto() *codesearchpb.SourceRange {
	return &codesearchpb.SourceRange{
		StartLine:   sourceRange.StartLine,
		StartColumn: sourceRange.StartColumn,
		EndLine:     sourceRange.EndLine,
		EndColumn:   sourceRange.EndColumn,
	}
}

func IndexStatusRequestFromProto(*codesearchpb.IndexStatusRequest) IndexStatusRequest {
	return IndexStatusRequest{}
}

func (IndexStatusRequest) ToProto() *codesearchpb.IndexStatusRequest {
	return &codesearchpb.IndexStatusRequest{}
}

func IndexStatusResponseFromProto(response *codesearchpb.IndexStatusResponse) IndexStatusResponse {
	if response == nil {
		return IndexStatusResponse{}
	}
	languages := make(map[string]int32, len(response.GetLanguages()))
	for language, count := range response.GetLanguages() {
		languages[language] = count
	}
	return IndexStatusResponse{
		FileCount:      response.GetFileCount(),
		TotalBytes:     response.GetTotalBytes(),
		IndexBytes:     response.GetIndexBytes(),
		EmbeddingCount: response.GetEmbeddingCount(),
		Languages:      languages,
	}
}

func (response IndexStatusResponse) ToProto() *codesearchpb.IndexStatusResponse {
	languages := make(map[string]int32, len(response.Languages))
	for language, count := range response.Languages {
		languages[language] = count
	}
	return &codesearchpb.IndexStatusResponse{
		FileCount:      response.FileCount,
		TotalBytes:     response.TotalBytes,
		IndexBytes:     response.IndexBytes,
		EmbeddingCount: response.EmbeddingCount,
		Languages:      languages,
	}
}
