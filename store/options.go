package store

// Filter narrows search, list, and lookup operations.
type Filter struct {
	RepositoryID string
	Branch       string
	PathPrefix   string
	Language     string
	DocumentID   string
	SymbolID     string
	Tier         Tier
	Kinds        []SymbolKind
	Metadata     map[string]string
}

// SearchOptions controls a search operation.
type SearchOptions struct {
	Filter Filter
	Limit  int
	Cursor string
}

// ListOptions controls a list operation.
type ListOptions struct {
	Filter Filter
	Limit  int
	Cursor string
}

// LookupOptions controls a lookup operation.
type LookupOptions struct {
	Filter Filter
}

// SearchOption configures SearchOptions.
type SearchOption interface {
	applySearch(*SearchOptions)
}

// ListOption configures ListOptions.
type ListOption interface {
	applyList(*ListOptions)
}

// LookupOption configures LookupOptions.
type LookupOption interface {
	applyLookup(*LookupOptions)
}

type option struct {
	search func(*SearchOptions)
	list   func(*ListOptions)
	lookup func(*LookupOptions)
}

func (o option) applySearch(opts *SearchOptions) {
	if o.search != nil {
		o.search(opts)
	}
}

func (o option) applyList(opts *ListOptions) {
	if o.list != nil {
		o.list(opts)
	}
}

func (o option) applyLookup(opts *LookupOptions) {
	if o.lookup != nil {
		o.lookup(opts)
	}
}

// ResolveSearchOptions applies search options and returns the resulting config.
func ResolveSearchOptions(opts ...SearchOption) SearchOptions {
	var out SearchOptions
	for _, opt := range opts {
		if opt != nil {
			opt.applySearch(&out)
		}
	}
	return out
}

// ResolveListOptions applies list options and returns the resulting config.
func ResolveListOptions(opts ...ListOption) ListOptions {
	var out ListOptions
	for _, opt := range opts {
		if opt != nil {
			opt.applyList(&out)
		}
	}
	return out
}

// ResolveLookupOptions applies lookup options and returns the resulting config.
func ResolveLookupOptions(opts ...LookupOption) LookupOptions {
	var out LookupOptions
	for _, opt := range opts {
		if opt != nil {
			opt.applyLookup(&out)
		}
	}
	return out
}

// WithFilter sets a complete filter on search, list, and lookup operations.
func WithFilter(filter Filter) option {
	filter = cloneFilter(filter)
	return option{
		search: func(opts *SearchOptions) {
			opts.Filter = cloneFilter(filter)
		},
		list: func(opts *ListOptions) {
			opts.Filter = cloneFilter(filter)
		},
		lookup: func(opts *LookupOptions) {
			opts.Filter = cloneFilter(filter)
		},
	}
}

// WithRepositoryID filters by repository ID.
func WithRepositoryID(repositoryID string) option {
	return option{
		search: func(opts *SearchOptions) {
			opts.Filter.RepositoryID = repositoryID
		},
		list: func(opts *ListOptions) {
			opts.Filter.RepositoryID = repositoryID
		},
		lookup: func(opts *LookupOptions) {
			opts.Filter.RepositoryID = repositoryID
		},
	}
}

// WithBranch filters by branch.
func WithBranch(branch string) option {
	return option{
		search: func(opts *SearchOptions) {
			opts.Filter.Branch = branch
		},
		list: func(opts *ListOptions) {
			opts.Filter.Branch = branch
		},
		lookup: func(opts *LookupOptions) {
			opts.Filter.Branch = branch
		},
	}
}

// WithPathPrefix filters by path prefix.
func WithPathPrefix(pathPrefix string) option {
	return option{
		search: func(opts *SearchOptions) {
			opts.Filter.PathPrefix = pathPrefix
		},
		list: func(opts *ListOptions) {
			opts.Filter.PathPrefix = pathPrefix
		},
		lookup: func(opts *LookupOptions) {
			opts.Filter.PathPrefix = pathPrefix
		},
	}
}

// WithLanguage filters by language.
func WithLanguage(language string) option {
	return option{
		search: func(opts *SearchOptions) {
			opts.Filter.Language = language
		},
		list: func(opts *ListOptions) {
			opts.Filter.Language = language
		},
		lookup: func(opts *LookupOptions) {
			opts.Filter.Language = language
		},
	}
}

// WithDocumentID filters by document ID.
func WithDocumentID(documentID string) option {
	return option{
		search: func(opts *SearchOptions) {
			opts.Filter.DocumentID = documentID
		},
		list: func(opts *ListOptions) {
			opts.Filter.DocumentID = documentID
		},
		lookup: func(opts *LookupOptions) {
			opts.Filter.DocumentID = documentID
		},
	}
}

// WithSymbolID filters by symbol ID.
func WithSymbolID(symbolID string) option {
	return option{
		search: func(opts *SearchOptions) {
			opts.Filter.SymbolID = symbolID
		},
		list: func(opts *ListOptions) {
			opts.Filter.SymbolID = symbolID
		},
		lookup: func(opts *LookupOptions) {
			opts.Filter.SymbolID = symbolID
		},
	}
}

// WithTier filters by storage tier.
func WithTier(tier Tier) option {
	return option{
		search: func(opts *SearchOptions) {
			opts.Filter.Tier = tier
		},
		list: func(opts *ListOptions) {
			opts.Filter.Tier = tier
		},
		lookup: func(opts *LookupOptions) {
			opts.Filter.Tier = tier
		},
	}
}

// WithKinds filters by symbol kind.
func WithKinds(kinds ...SymbolKind) option {
	copied := append([]SymbolKind(nil), kinds...)
	return option{
		search: func(opts *SearchOptions) {
			opts.Filter.Kinds = append([]SymbolKind(nil), copied...)
		},
		list: func(opts *ListOptions) {
			opts.Filter.Kinds = append([]SymbolKind(nil), copied...)
		},
		lookup: func(opts *LookupOptions) {
			opts.Filter.Kinds = append([]SymbolKind(nil), copied...)
		},
	}
}

// WithMetadata adds a metadata key/value filter.
func WithMetadata(key, value string) option {
	return option{
		search: func(opts *SearchOptions) {
			if opts.Filter.Metadata == nil {
				opts.Filter.Metadata = make(map[string]string)
			}
			opts.Filter.Metadata[key] = value
		},
		list: func(opts *ListOptions) {
			if opts.Filter.Metadata == nil {
				opts.Filter.Metadata = make(map[string]string)
			}
			opts.Filter.Metadata[key] = value
		},
		lookup: func(opts *LookupOptions) {
			if opts.Filter.Metadata == nil {
				opts.Filter.Metadata = make(map[string]string)
			}
			opts.Filter.Metadata[key] = value
		},
	}
}

// WithLimit caps the number of results returned from search and list operations.
func WithLimit(limit int) option {
	return option{
		search: func(opts *SearchOptions) {
			opts.Limit = limit
		},
		list: func(opts *ListOptions) {
			opts.Limit = limit
		},
	}
}

// WithCursor sets the pagination cursor for search and list operations.
func WithCursor(cursor string) option {
	return option{
		search: func(opts *SearchOptions) {
			opts.Cursor = cursor
		},
		list: func(opts *ListOptions) {
			opts.Cursor = cursor
		},
	}
}

func cloneFilter(filter Filter) Filter {
	clone := filter
	if len(filter.Kinds) != 0 {
		clone.Kinds = append([]SymbolKind(nil), filter.Kinds...)
	}
	if len(filter.Metadata) != 0 {
		clone.Metadata = make(map[string]string, len(filter.Metadata))
		for key, value := range filter.Metadata {
			clone.Metadata[key] = value
		}
	}
	return clone
}
