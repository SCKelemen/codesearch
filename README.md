# codesearch

A high-performance, language-aware code search library written in Go.

`codesearch` is a systems-oriented library for building local or embedded code
search experiences with serious indexing and code-intelligence primitives. It
combines fast lexical retrieval, optional semantic ranking, structural symbol
extraction, LSIF generation, and a transport layer for serving search over HTTP
and ConnectRPC.

The repository includes both the core Go packages and `csx`, an interactive CLI
for indexing, searching, serving, and inspecting code indexes.

## Features

- Trigram indexing for fast substring search
- Fuzzy matching (fzf-inspired algorithm)
- Exact match with case-insensitive support
- Hybrid search combining multiple strategies
- Language-aware structural symbol extraction (Go, TypeScript, JavaScript, Python, Rust, SQL)
- LSP multiplexer for compiler-quality code intelligence (14 language servers)
- LSIF 0.4.3 generator for portable code intelligence
- CEL-based query language for structured filtering
- ConnectRPC service layer
- Pluggable store abstraction (memory, file-backed, shard format)
- Configurable ranking and scoring
- Interactive TUI CLI (csx)

## Architecture

The library is organized as a set of focused packages that can be used together
through the root engine or independently in more specialized applications.

```text
codesearch (root engine)
├── trigram/      - Trigram indexing and search
├── fuzzy/        - Fuzzy matching (fzf algorithm)
├── exact/        - Exact substring search
├── hybrid/       - Multi-strategy search combiner
├── ranking/      - Result scoring and ranking
├── search/       - Search orchestration
├── index/        - Document indexing pipeline
├── content/      - Language detection, binary filtering
├── linguist/     - GitHub Linguist language colors
├── gitlog/       - Git commit/trailer parsing
├── structural/   - Language-aware symbol extraction
├── celfilter/    - CEL query evaluation
├── symbol/       - Symbol types and indexing
├── embedding/    - Embedding interfaces for semantic search
├── shard/        - Shard format for distributed indexes
├── store/        - Storage abstraction layer
│   ├── memory/   - In-memory store
│   └── file/     - File-backed persistent store
├── lsp/          - LSP JSON-RPC 2.0 client + multiplexer
│   └── lsifgen/  - LSIF generator from LSP queries
├── query/        - Query parsing and planning
├── proto/        - ConnectRPC service definitions
│   └── codesearchv1/  - Generated stubs + handler
├── gen/          - Generated protobuf code
└── cmd/csx/      - CLI tool
```

### Search pipeline at a glance

At the top level, `codesearch.Engine` coordinates four main concerns:

1. **Indexing**: files are normalized, language-tagged, and stored in a
   pluggable backend.
2. **Lexical retrieval**: trigram postings narrow the candidate set before
   exact or regex-like matching confirms hits.
3. **Structural enrichment**: symbol extraction adds definitions, containers,
   kinds, and export metadata for symbol-aware workflows.
4. **Fusion and serving**: lexical, semantic, and structural results can be
   ranked, filtered, and exposed over CLI, JSON, or ConnectRPC.

This architecture makes the project suitable both as an embeddable library and
as the foundation for a standalone code search service.

## Quick Start

### Install the library

```bash
go get github.com/SCKelemen/codesearch
```

### Basic indexing

Use a file-backed engine when you want a reusable on-disk index:

```go
package main

import (
	"context"
	"log"

	"github.com/SCKelemen/codesearch"
)

func main() {
	ctx := context.Background()

	engine, err := codesearch.Open("./index")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = engine.Close()
	}()

	if err := engine.Index(ctx, "./cmd/csx"); err != nil {
		log.Fatal(err)
	}
}
```

For ephemeral use cases, `codesearch.New()` creates an in-memory engine.

### Basic search

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/SCKelemen/codesearch"
)

func main() {
	ctx := context.Background()

	engine, err := codesearch.Open("./index")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = engine.Close()
	}()

	results, err := engine.Search(
		ctx,
		"multiplexer",
		codesearch.WithLimit(10),
		codesearch.WithFilter(`language == "go"`),
	)
	if err != nil {
		log.Fatal(err)
	}

	for _, result := range results {
		fmt.Printf("%s:%d %.3f\n", result.Path, result.Line, result.Score)
		fmt.Println(result.Snippet)
	}
}
```

## Querying

### Search modes

Every search runs in one of three modes, selected with `--mode` (CLI) or
`codesearch.WithMode()` (library):

| Mode | Flag | Description |
|------|------|-------------|
| **Hybrid** | `--mode hybrid` | Runs all configured backends (lexical + semantic) and fuses the results. Default. |
| **Lexical** | `--mode lexical` | Trigram posting lists narrow candidates, then exact/regex matching confirms hits. Fast, precise, no embeddings required. |
| **Semantic** | `--mode semantic` | Vector similarity search over document embeddings. Finds conceptually related code even when keywords differ. Requires `--embeddings` at index time. |

**When to use which:**

- **Lexical** — exact identifier lookup, error messages, import paths, known strings
- **Semantic** — "find code that handles authentication", "functions similar to X"
- **Hybrid** — best of both; the default for most workflows

### CEL filter expressions

Results can be filtered with [CEL](https://github.com/google/cel-spec)
(Common Expression Language) expressions. Filters run server-side and reduce
results before they are returned.

#### CLI usage

```bash
csx search "handleRequest" --filter 'language == "go"'
csx search "TODO" --filter 'file_extension == ".ts" && file_size < 5000'
csx search "auth" --filter 'repository == "SCKelemen/codesearch"'
csx search "fix" --filter 'file_path.startsWith("cmd/")'
```

#### Library usage

```go
results, err := engine.Search(ctx, "handleRequest",
    codesearch.WithFilter(`language == "go" && file_size < 10000`),
    codesearch.WithLimit(20),
)
```

#### Available filter variables

##### File attributes

| Variable | Type | Description | Example |
|----------|------|-------------|---------|
| `language` | `string` | Normalized language name (lowercase) | `"go"`, `"typescript"`, `"python"` |
| `file_path` | `string` | Full file path or URI | `"cmd/csx/search.go"` |
| `file_extension` | `string` | Lowercase file extension including dot | `".go"`, `".ts"`, `".py"` |
| `file_size` | `int` | File size in bytes | `2048` |

##### Repository attributes

| Variable | Type | Description | Example |
|----------|------|-------------|---------|
| `branch` | `string` | Git branch name | `"main"`, `"develop"` |
| `repository` | `string` | Repository identifier (owner/name) | `"SCKelemen/codesearch"` |
| `project_id` | `string` | Project identifier (when set) | `"proj_123"` |

##### Commit attributes

Available when `--git` is used during indexing (`csx index --git`):

| Variable | Type | Description | Example |
|----------|------|-------------|---------|
| `commit_author` | `string` | Author email of the last commit | `"sam@example.com"` |
| `commit_author_name` | `string` | Author name of the last commit | `"Sam"` |
| `commit_committer` | `string` | Committer email | `"ci@example.com"` |
| `commit_date` | `timestamp` | Author date of the last commit | `timestamp("2025-01-15T10:30:00Z")` |
| `commit_message` | `string` | Subject line of the last commit | `"fix: resolve auth bug"` |
| `commit_coauthors` | `list(string)` | Co-author emails from trailers | `["alex@example.com"]` |
| `commit_source` | `string` | Source platform | `"github"` |
| `commit_is_agent` | `bool` | Whether the commit was made by an agent | `false` |

#### CEL operators and functions

CEL supports standard operators and string functions:

```bash
# Equality and comparison
--filter 'language == "go"'
--filter 'file_size > 1000 && file_size < 50000'
--filter 'file_size >= 1024'

# Logical operators
--filter 'language == "go" || language == "rust"'
--filter '!(language == "markdown")'
--filter 'language == "typescript" && branch == "main"'

# String functions
--filter 'file_path.startsWith("cmd/")'
--filter 'file_path.endsWith("_test.go")'
--filter 'file_path.contains("internal")'
--filter 'commit_message.contains("fix")'

# List membership
--filter '"sam@example.com" in commit_coauthors'

# Timestamp comparison
--filter 'commit_date > timestamp("2025-01-01T00:00:00Z")'
--filter 'commit_date > timestamp("2025-06-01T00:00:00Z") && language == "go"'

# Negation
--filter 'file_extension != ".md" && file_extension != ".txt"'
```

#### Filter examples by use case

**Find recently modified Go files:**
```bash
csx search "error" --filter 'language == "go" && commit_date > timestamp("2025-06-01T00:00:00Z")'
```

**Find large files in a specific directory:**
```bash
csx search "config" --filter 'file_path.startsWith("internal/") && file_size > 10000'
```

**Find test files across languages:**
```bash
csx search "assert" --filter 'file_path.endsWith("_test.go") || file_path.endsWith(".test.ts")'
```

**Find code by a specific author:**
```bash
csx search "refactor" --filter 'commit_author_name == "Sam"'
```

**Exclude generated code:**
```bash
csx search "func" --filter '!file_path.contains("generated") && !file_path.contains("vendor")'
```

**Search across a specific repository and branch:**
```bash
csx search "deploy" --filter 'repository == "SCKelemen/codesearch" && branch == "main"'
```

### Context lines

Search results show surrounding source lines for context, similar to
`grep -C`. Control the number of lines with `--context` / `-C`:

```bash
csx search "func" --context 2   # 2 lines above + 2 below (default)
csx search "func" --context 0   # match line only
csx search "func" --context 5   # 5 lines above + 5 below
```

Output format:

```
 1. cmd/csx/search.go:42                                              0.016
    40 | }
    41 |
    42 | func runSearch(ctx *clix.Context, request searchRequest) ...
    43 |     mode, err := normalizeMode(request.Mode)
    44 |     if err != nil {
```

- **Match line**: highlighted with accent color and match range markers
- **Context lines**: muted color with aligned line numbers
- **Line numbers**: right-aligned, separated by pipe

### Git metadata enrichment

When indexing with `--git`, each document is enriched with metadata from the
`.git` directory. This metadata powers the commit-related CEL filter variables.

```bash
csx index . --git --output ./index
```

The indexer extracts:

| Metadata | Source | Stored as |
|----------|--------|-----------|
| Repository name | `git remote` | `repository` field + document `RepositoryID` |
| Branch | `git rev-parse` | `branch` field + document `Branch` |
| Last commit SHA | `git log -1` | `commit_sha` metadata |
| Author name | `git log -1` | `author_name` metadata |
| Author email | `git log -1` | `author_email` metadata |
| Commit message | `git log -1` | `commit_message` metadata |
| Last modified date | `git log -1` | `last_modified` metadata |
| First commit date | `git log --reverse` | `first_committed` metadata |
| Commit count | `git rev-list --count` | `commit_count` metadata |

### Using the CLI

Install the CLI:

```bash
go install github.com/SCKelemen/codesearch/cmd/csx@latest
```

Index a repository:

```bash
csx index . --output ./index
```

Search the index:

```bash
csx search "multiplexer" --index ./index
```

Launch the interactive TUI:

```bash
csx interactive --index ./index
```

If LSP-backed symbol extraction and LSIF generation are available in your
environment, enable them with the global `--lsp` flag:

```bash
csx --lsp index . --output ./index
```

## CLI Usage

The `csx` executable is the operational front end for the repository. It wraps
local indexing, interactive search, service hosting, and LSIF export in a
single tool.

### `csx index`

Build a local index for a file or directory.

```bash
csx index <path> [--output ./index] [--language go,ts] [--embeddings] [--git]
```

Examples:

```bash
csx index . --output ./index
csx index ./cmd/csx --output ./index --language go
csx --lsp index . --output ./index --embeddings
csx index . --output ./index --git
```

Notes:

- Skips common vendor-like directories such as `.git`, `node_modules`, and `vendor`
- Filters binary files
- Can emit `index.lsif` alongside the local index when `--lsp` is enabled
- Supports deterministic local embeddings when an embedder is configured

### `csx search`

Search a local index or a remote service.

```bash
csx search [query] [--index ./index] [--limit 20] [--mode hybrid|lexical|semantic] [--filter <cel>] [--context N] [--json] [--remote <addr>]
```

Examples:

```bash
csx search "NewService" --index ./index
csx search "hoverResult" --index ./index --mode lexical --json
csx search "symbol extraction" --remote 127.0.0.1:8080
csx search "handleRequest" --filter 'language == "go"' --context 3
```

Notes:

- Omitting the query launches interactive search
- `--mode` supports lexical, semantic, and hybrid retrieval
- `--filter` accepts CEL expressions (see [Querying](#querying) for the full variable reference)
- `--context` / `-C` controls surrounding lines shown (default 2, use 0 for match only)
- `--remote` first attempts ConnectRPC and then falls back to a JSON endpoint

### `csx serve`

Serve a local index over HTTP and ConnectRPC.

```bash
csx serve [--addr :8080] [--index ./index]
```

Examples:

```bash
csx serve --index ./index --addr :8080
curl 'http://127.0.0.1:8080/api/search?q=multiplexer&mode=lexical&limit=10'
```

The server exposes:

- `GET /api/search` for JSON search responses
- ConnectRPC procedures for search, index status, and symbol search


### `csx github`

Index your GitHub repositories for local code search. Downloads repos via the
GitHub Archive API (tarball) for maximum speed, with automatic fallback to
parallel per-file fetching.

#### Authentication

`csx github` needs a GitHub token. Three ways to provide one:

**Option 1: Use the `gh` CLI (recommended)**

If you have the [GitHub CLI](https://cli.github.com/) installed and authenticated:

```bash
# Authenticate once (if you haven't already)
gh auth login

# csx will use your gh token automatically
csx github
```

Under the hood, `csx` checks `GITHUB_TOKEN`, then `GH_TOKEN`. You can
export the token from `gh`:

```bash
export GITHUB_TOKEN=$(gh auth token)
csx github
```

**Option 2: Pass a token directly**

```bash
csx github --token ghp_yourPersonalAccessToken
```

**Option 3: Set an environment variable**

```bash
export GITHUB_TOKEN=ghp_yourPersonalAccessToken
# or
export GH_TOKEN=ghp_yourPersonalAccessToken

csx github
```

> **Required scopes**: `repo` (for private repos) or no scopes needed for public repos only.
> Check your token scopes with `gh auth status`.

#### Examples

```bash
# Index all your repos (authenticated user)
csx github

# Index a specific user's public repos (Go and TypeScript only)
csx github --user SCKelemen --language go,ts

# Index an organization's repos
csx github --org lovablelabs --max-repos 20

# Index with a custom output directory
csx github --output ~/.csx/my-repos

# Include archived and forked repos
csx github --archived --forks

# Search across all indexed repos
csx search --index .csx/github "func handleRequest"

# One-liner: index and search using gh CLI token
GITHUB_TOKEN=$(gh auth token) csx github --user SCKelemen --max-repos 5 && csx search --index .csx/github "TODO"
```

#### How it works

1. **Discover** — Lists repos via the GitHub API (sorted by most recently updated)
2. **Download** — Fetches each repo as a gzipped tarball (1 HTTP request per repo)
3. **Filter** — Skips lockfiles, vendor dirs, minified assets, and files >1 MiB
4. **Index** — Builds trigram, symbol, and vector indexes into a shared local store
5. **URI** — Each file gets a `github://owner/repo/path@branch` URI for search results

If the tarball API is unavailable, falls back to parallel per-file fetching via
the Contents API (`--concurrency` controls the worker count, default 8).


### `csx pierre`

Index repositories from a [Pierre/code.storage](https://code.storage) instance.
Uses the archive API for fast single-request downloads, with automatic fallback
to parallel per-file fetching via the file API.

#### Authentication

Pierre requires a base URL and an API token. See the
[Pierre authentication docs](https://code.storage/docs/getting-started/authentication)
for how to obtain credentials.

**Option 1: Environment variables (recommended)**

```bash
export PIERRE_URL=https://lovable.code.storage
export PIERRE_TOKEN=your-api-token
csx pierre
```

**Option 2: CLI flags**

```bash
csx pierre --url https://lovable.code.storage --token your-api-token
```

#### Examples

```bash
# Index all repos from a Pierre instance
csx pierre --url https://lovable.code.storage --token $PIERRE_TOKEN

# Index a specific repo
csx pierre --repo my-project

# Index a specific branch
csx pierre --repo my-project --branch develop

# Filter by language
csx pierre --language go,ts,js

# Limit number of repos
csx pierre --max-repos 10

# Custom output directory
csx pierre --output ~/.csx/pierre-repos

# Search indexed Pierre repos
csx search --index .csx/pierre "func handleRequest"
```

#### How it works

1. **Discover** — Lists repositories via `GET /api/v1/repos`
2. **Resolve branch** — Uses `--branch`, repo default branch, or `main`
3. **Download** — Fetches `GET /api/v1/repos/{id}/archive?ref={ref}` (tar.gz).
   Falls back to `GET /api/v1/repos/{id}/files` + parallel
   `GET /api/v1/repos/{id}/file/{path}` if archive is unavailable
4. **Filter** — Skips binaries, vendor dirs, files over 1 MB
5. **Index** — Builds trigram + content indexes with URI scheme
   `pierre://{repo}/{path}@{ref}`

#### Pierre API endpoints used

| Endpoint | Purpose |
|----------|---------|
| `GET /api/v1/repos` | List available repositories |
| `GET /api/v1/repos/{id}/branches` | List branches (for future multi-branch indexing) |
| `GET /api/v1/repos/{id}/archive?ref=` | Download repo as tar.gz (primary path) |
| `GET /api/v1/repos/{id}/files?ref=` | List files with metadata (fallback path) |
| `GET /api/v1/repos/{id}/file/{path}?ref=` | Fetch individual file content (fallback path) |
| `GET /api/v1/repos/{id}/grep` | Server-side grep (reserved for future hybrid search) |

#### Webhooks (Lovable platform integration)

When running as part of the Lovable platform, Pierre
[webhooks](https://code.storage/docs/guides/webhooks) trigger automatic
re-indexing on push events. The `csx` CLI indexes on-demand; the Lovable
service layer uses webhooks for real-time index freshness via Temporal workflows.

### `csx lsif`

Generate LSIF JSON Lines output from LSP-backed analysis.

```bash
csx lsif <path> [--output <file>]
```

Examples:

```bash
csx lsif . --output ./index/index.lsif
csx lsif ./cmd/csx
```

### `csx interactive`

Open the full-screen terminal interface.

```bash
csx interactive [--index ./index]
```

This mode is useful when you want to browse ranked results quickly, inspect
snippets, and open a preview without switching tools.

## LSP Support

`codesearch` ships with a built-in LSP multiplexer that starts any supported
server present on `PATH` and routes files to the best available client.

| Server ID | Command | Primary file types |
| --- | --- | --- |
| `typescript` | `typescript-language-server --stdio` | `.ts`, `.tsx`, `.js`, `.jsx`, `.mjs`, `.mts` |
| `go` | `gopls` | `.go` |
| `rust` | `rust-analyzer` | `.rs` |
| `python` | `pyright-langserver --stdio` | `.py` |
| `html` | `vscode-html-language-server --stdio` | `.html`, `.htm` |
| `css` | `vscode-css-language-server --stdio` | `.css`, `.scss`, `.less` |
| `sql` | `sql-language-server up --method stdio` | `.sql`, `.pgsql` |
| `postgres` | `postgrestools lsp` | `.sql`, `.pgsql` |
| `prisma` | `prisma-language-server --stdio` | `.prisma` |
| `json` | `vscode-json-language-server --stdio` | `.json`, `.jsonc` |
| `toml` | `taplo lsp stdio` | `.toml` |
| `tailwind` | `tailwindcss-language-server --stdio` | `.css`, `.scss`, `.less`, `.html`, `.tsx`, `.jsx` |
| `eslint` | `vscode-eslint-language-server --stdio` | `.js`, `.jsx`, `.ts`, `.tsx`, `.mjs`, `.mts` |
| `biome` | `biome lsp-proxy` | `.js`, `.jsx`, `.ts`, `.tsx`, `.json`, `.jsonc` |

### What LSP mode adds

When enabled, the LSP stack improves the quality of symbol extraction and makes
additional intelligence available for LSIF export, including:

- document symbols
- definitions
- references
- hover information
- language-specific ranges and containers

## Benchmarks

Run the full benchmark suite with Go's built-in benchmarking support:

```bash
go test ./... -bench=. -benchmem
```

This repository includes focused benchmarks for core subsystems such as trigram
search, exact search, fuzzy matching, ranking, indexing, search orchestration,
and shard handling.

## Security

LSP-derived data should be treated as trusted build-time or indexing-time input,
not untrusted user input.

**Important:** LSP data must come from trusted infrastructure, never from
user-controlled environments.

Practical implications:

- run language servers in controlled development or CI environments
- do not accept arbitrary LSIF or LSP responses from end users
- isolate index generation from hostile repositories when operating a shared service
- validate deployment boundaries if search indexes are built from multi-tenant sources

## License

No `LICENSE` file is currently present in this repository.

Until a license is added, you should assume the code is **not** available for
unrestricted reuse or redistribution. If you intend to publish or consume this
project as a dependency outside its current environment, add an explicit license
file first.
