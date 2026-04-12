package lsp

import "testing"

func TestDefaultConfigs(t *testing.T) {
	configs := DefaultConfigs()

	expected := map[ServerID]struct{}{
		ServerTypeScript: {},
		ServerGo:         {},
		ServerRust:       {},
		ServerPython:     {},
		ServerHTML:       {},
		ServerCSS:        {},
		ServerSQL:        {},
		ServerPostgres:   {},
		ServerPrisma:     {},
		ServerJSON:       {},
		ServerTOML:       {},
		ServerTailwind:   {},
		ServerESLint:     {},
		ServerBiome:      {},
	}

	if len(configs) != len(expected) {
		t.Fatalf("DefaultConfigs() returned %d configs, want %d", len(configs), len(expected))
	}

	seen := make(map[ServerID]bool, len(configs))
	for _, cfg := range configs {
		if _, ok := expected[cfg.ID]; !ok {
			t.Fatalf("DefaultConfigs() returned unexpected server ID %q", cfg.ID)
		}
		if seen[cfg.ID] {
			t.Fatalf("DefaultConfigs() returned duplicate server ID %q", cfg.ID)
		}
		seen[cfg.ID] = true
		if len(cfg.Command) == 0 {
			t.Fatalf("server %q has empty command", cfg.ID)
		}
		if cfg.LanguageID == "" {
			t.Fatalf("server %q has empty language ID", cfg.ID)
		}
	}

	for id := range expected {
		if !seen[id] {
			t.Fatalf("DefaultConfigs() missing server ID %q", id)
		}
	}
}

func TestLangIDForExt(t *testing.T) {
	tests := map[string]string{
		".ts":     "typescript",
		".tsx":    "typescriptreact",
		".js":     "javascript",
		".jsx":    "javascriptreact",
		".mjs":    "javascript",
		".mts":    "typescript",
		".go":     "go",
		".rs":     "rust",
		".py":     "python",
		".html":   "html",
		".htm":    "html",
		".css":    "css",
		".scss":   "scss",
		".less":   "less",
		".sql":    "sql",
		".pgsql":  "sql",
		".prisma": "prisma",
		".json":   "json",
		".jsonc":  "jsonc",
		".toml":   "toml",
		"ts":      "typescript",
		"":        "",
		".txt":    "",
	}

	for ext, want := range tests {
		if got := LangIDForExt(ext); got != want {
			t.Fatalf("LangIDForExt(%q) = %q, want %q", ext, got, want)
		}
	}
}

func TestClientForFile(t *testing.T) {
	tsClient := &Client{id: ServerTypeScript}
	goClient := &Client{id: ServerGo}
	rustClient := &Client{id: ServerRust}
	pythonClient := &Client{id: ServerPython}
	htmlClient := &Client{id: ServerHTML}
	cssClient := &Client{id: ServerCSS}
	sqlClient := &Client{id: ServerSQL}
	postgresClient := &Client{id: ServerPostgres}
	prismaClient := &Client{id: ServerPrisma}
	jsonClient := &Client{id: ServerJSON}
	tomlClient := &Client{id: ServerTOML}
	tailwindClient := &Client{id: ServerTailwind}

	mux := &Multiplexer{
		clients: map[ServerID]*Client{
			ServerTypeScript: tsClient,
			ServerGo:         goClient,
			ServerRust:       rustClient,
			ServerPython:     pythonClient,
			ServerHTML:       htmlClient,
			ServerCSS:        cssClient,
			ServerSQL:        sqlClient,
			ServerPostgres:   postgresClient,
			ServerPrisma:     prismaClient,
			ServerJSON:       jsonClient,
			ServerTOML:       tomlClient,
			ServerTailwind:   tailwindClient,
		},
	}

	tests := []struct {
		name string
		path string
		want *Client
	}{
		{name: "typescript", path: "file.ts", want: tsClient},
		{name: "tsx", path: "file.tsx", want: tsClient},
		{name: "javascript", path: "file.js", want: tsClient},
		{name: "jsx", path: "file.jsx", want: tsClient},
		{name: "mjs", path: "file.mjs", want: tsClient},
		{name: "mts", path: "file.mts", want: tsClient},
		{name: "go", path: "file.go", want: goClient},
		{name: "rust", path: "file.rs", want: rustClient},
		{name: "python", path: "file.py", want: pythonClient},
		{name: "html", path: "file.html", want: htmlClient},
		{name: "htm", path: "file.htm", want: htmlClient},
		{name: "css prefers tailwind", path: "file.css", want: tailwindClient},
		{name: "scss prefers tailwind", path: "file.scss", want: tailwindClient},
		{name: "less prefers tailwind", path: "file.less", want: tailwindClient},
		{name: "sql prefers postgres", path: "file.sql", want: postgresClient},
		{name: "pgsql prefers postgres", path: "file.pgsql", want: postgresClient},
		{name: "prisma", path: "schema.prisma", want: prismaClient},
		{name: "json", path: "file.json", want: jsonClient},
		{name: "jsonc", path: "file.jsonc", want: jsonClient},
		{name: "toml", path: "file.toml", want: tomlClient},
		{name: "default fallback", path: "file.txt", want: tsClient},
	}

	for _, tt := range tests {
		if got := mux.ClientForFile(tt.path); got != tt.want {
			t.Fatalf("%s: ClientForFile(%q) = %p, want %p", tt.name, tt.path, got, tt.want)
		}
	}

	fallbackMux := &Multiplexer{
		clients: map[ServerID]*Client{
			ServerTypeScript: tsClient,
			ServerCSS:        cssClient,
			ServerSQL:        sqlClient,
		},
	}

	if got := fallbackMux.ClientForFile("file.css"); got != cssClient {
		t.Fatalf("ClientForFile(css fallback) = %p, want %p", got, cssClient)
	}
	if got := fallbackMux.ClientForFile("file.sql"); got != sqlClient {
		t.Fatalf("ClientForFile(sql fallback) = %p, want %p", got, sqlClient)
	}
	if got := fallbackMux.ClientForFile("file.go"); got != tsClient {
		t.Fatalf("ClientForFile(go fallback) = %p, want %p", got, tsClient)
	}
}
