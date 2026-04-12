package lsp

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ServerID identifies a configured language server.
type ServerID string

const (
	ServerTypeScript ServerID = "typescript"
	ServerGo         ServerID = "go"
	ServerRust       ServerID = "rust"
	ServerPython     ServerID = "python"
	ServerHTML       ServerID = "html"
	ServerCSS        ServerID = "css"
	ServerSQL        ServerID = "sql"
	ServerPostgres   ServerID = "postgres"
	ServerPrisma     ServerID = "prisma"
	ServerJSON       ServerID = "json"
	ServerTOML       ServerID = "toml"
	ServerTailwind   ServerID = "tailwind"
	ServerESLint     ServerID = "eslint"
	ServerBiome      ServerID = "biome"
)

// ServerConfig describes a language-server configuration.
type ServerConfig struct {
	ID         ServerID
	Command    []string
	LanguageID string
	Extensions []string
}

// Multiplexer routes a file to the most appropriate running language server.
type Multiplexer struct {
	workDir string
	clients map[ServerID]*Client
}

// NewMultiplexer constructs a multiplexer rooted at the provided working directory.
func NewMultiplexer(workDir string) *Multiplexer {
	return &Multiplexer{
		workDir: workDir,
		clients: make(map[ServerID]*Client),
	}
}

// ConnectAvailable starts every configured language server that is available on PATH.
func (m *Multiplexer) ConnectAvailable(ctx context.Context) {
	if m == nil {
		return
	}
	if m.clients == nil {
		m.clients = make(map[ServerID]*Client)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	for _, cfg := range DefaultConfigs() {
		if len(cfg.Command) == 0 {
			continue
		}
		if _, err := exec.LookPath(cfg.Command[0]); err != nil {
			continue
		}
		if _, ok := m.clients[cfg.ID]; ok {
			continue
		}

		client, err := NewClient(ctx, m.workDir, cfg.Command)
		if err != nil {
			continue
		}
		client.id = cfg.ID
		m.clients[cfg.ID] = client
	}
}

// Close stops every connected language server.
func (m *Multiplexer) Close() error {
	if m == nil {
		return nil
	}

	var errs []error
	for id, client := range m.clients {
		if client == nil {
			continue
		}
		if err := client.Close(); err != nil {
			errs = append(errs, err)
		}
		delete(m.clients, id)
	}
	return errors.Join(errs...)
}

// ConnectedServerIDs returns the IDs of all connected servers in sorted order.
func (m *Multiplexer) ConnectedServerIDs() []ServerID {
	if m == nil || len(m.clients) == 0 {
		return nil
	}

	ids := make([]ServerID, 0, len(m.clients))
	for id, client := range m.clients {
		if client == nil {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return ids
}

// DefaultConfigs returns the built-in server configurations.
func DefaultConfigs() []ServerConfig {
	return []ServerConfig{
		{ID: ServerTypeScript, Command: []string{"typescript-language-server", "--stdio"}, LanguageID: "typescript", Extensions: []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".mts"}},
		{ID: ServerGo, Command: []string{"gopls"}, LanguageID: "go", Extensions: []string{".go"}},
		{ID: ServerRust, Command: []string{"rust-analyzer"}, LanguageID: "rust", Extensions: []string{".rs"}},
		{ID: ServerPython, Command: []string{"pyright-langserver", "--stdio"}, LanguageID: "python", Extensions: []string{".py"}},
		{ID: ServerHTML, Command: []string{"vscode-html-language-server", "--stdio"}, LanguageID: "html", Extensions: []string{".html", ".htm"}},
		{ID: ServerCSS, Command: []string{"vscode-css-language-server", "--stdio"}, LanguageID: "css", Extensions: []string{".css", ".scss", ".less"}},
		{ID: ServerSQL, Command: []string{"sql-language-server", "up", "--method", "stdio"}, LanguageID: "sql", Extensions: []string{".sql", ".pgsql"}},
		{ID: ServerPostgres, Command: []string{"postgrestools", "lsp"}, LanguageID: "sql", Extensions: []string{".sql", ".pgsql"}},
		{ID: ServerPrisma, Command: []string{"prisma-language-server", "--stdio"}, LanguageID: "prisma", Extensions: []string{".prisma"}},
		{ID: ServerJSON, Command: []string{"vscode-json-language-server", "--stdio"}, LanguageID: "json", Extensions: []string{".json", ".jsonc"}},
		{ID: ServerTOML, Command: []string{"taplo", "lsp", "stdio"}, LanguageID: "toml", Extensions: []string{".toml"}},
		{ID: ServerTailwind, Command: []string{"tailwindcss-language-server", "--stdio"}, LanguageID: "tailwindcss", Extensions: []string{".css", ".scss", ".less", ".html", ".tsx", ".jsx"}},
		{ID: ServerESLint, Command: []string{"vscode-eslint-language-server", "--stdio"}, LanguageID: "javascript", Extensions: []string{".js", ".jsx", ".ts", ".tsx", ".mjs", ".mts"}},
		{ID: ServerBiome, Command: []string{"biome", "lsp-proxy"}, LanguageID: "javascript", Extensions: []string{".js", ".jsx", ".ts", ".tsx", ".json", ".jsonc"}},
	}
}

// LangIDForExt returns the LSP language ID for a file extension.
func LangIDForExt(ext string) string {
	normalized := strings.ToLower(ext)
	if normalized == "" {
		return ""
	}
	if !strings.HasPrefix(normalized, ".") {
		normalized = "." + normalized
	}
	switch normalized {
	case ".ts", ".mts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js", ".mjs":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".py":
		return "python"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	case ".less":
		return "less"
	case ".sql", ".pgsql":
		return "sql"
	case ".prisma":
		return "prisma"
	case ".json":
		return "json"
	case ".jsonc":
		return "jsonc"
	case ".toml":
		return "toml"
	default:
		return ""
	}
}

// ClientForFile returns the best client for the given file path.
func (m *Multiplexer) ClientForFile(path string) *Client {
	if m == nil || len(m.clients) == 0 {
		return nil
	}
	choose := func(ids ...ServerID) *Client {
		for _, id := range ids {
			if client := m.clients[id]; client != nil {
				return client
			}
		}
		return nil
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".mts":
		if client := choose(ServerTypeScript, ServerBiome, ServerESLint); client != nil {
			return client
		}
	case ".go":
		if client := choose(ServerGo); client != nil {
			return client
		}
	case ".rs":
		if client := choose(ServerRust); client != nil {
			return client
		}
	case ".py":
		if client := choose(ServerPython); client != nil {
			return client
		}
	case ".html", ".htm":
		if client := choose(ServerHTML); client != nil {
			return client
		}
	case ".css", ".scss", ".less":
		if client := choose(ServerTailwind, ServerCSS); client != nil {
			return client
		}
	case ".sql", ".pgsql":
		if client := choose(ServerPostgres, ServerSQL); client != nil {
			return client
		}
	case ".prisma":
		if client := choose(ServerPrisma); client != nil {
			return client
		}
	case ".json", ".jsonc":
		if client := choose(ServerJSON); client != nil {
			return client
		}
	case ".toml":
		if client := choose(ServerTOML); client != nil {
			return client
		}
	}

	return choose(ServerTypeScript, ServerBiome, ServerESLint)
}
