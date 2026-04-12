# codesearch

Generic code search and indexing library.

## Packages

- `trigram/` - Trigram index and search engine
- `content/` - URI-based content resolution interface
- `linguist/` - Programming language detection and metadata
- `gitlog/` - Git log parsing and trailer extraction

## Design

All content is addressed by URI. The `content.Resolver` interface
allows plugging in any URI scheme (local files, HTTP, custom protocols).

LSIF/LSP intelligence is in [github.com/SCKelemen/lsp](https://github.com/SCKelemen/lsp).
