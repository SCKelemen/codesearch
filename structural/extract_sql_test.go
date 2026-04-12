package structural

import "testing"

func TestExtractSQLSymbols(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		path  string
		src   string
		wants []Symbol
	}{
		{
			name: "basic create table with columns",
			path: "schema.sql",
			src: `CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    email TEXT NOT NULL,
    created_at TIMESTAMP
);`,
			wants: []Symbol{
				{Name: "users", Kind: SymbolKindStruct, Language: "sql", Path: "schema.sql"},
				{Name: "id", Kind: SymbolKindField, Language: "sql", Path: "schema.sql", Container: "users"},
				{Name: "email", Kind: SymbolKindField, Language: "sql", Path: "schema.sql", Container: "users"},
				{Name: "created_at", Kind: SymbolKindField, Language: "sql", Path: "schema.sql", Container: "users"},
			},
		},
		{
			name:  "create view",
			path:  "view.sql",
			src:   `CREATE OR REPLACE VIEW active_users AS SELECT * FROM users;`,
			wants: []Symbol{{Name: "active_users", Kind: SymbolKindType, Language: "sql", Path: "view.sql"}},
		},
		{
			name:  "create index",
			path:  "index.sql",
			src:   `CREATE UNIQUE INDEX idx_users_email ON users (email);`,
			wants: []Symbol{{Name: "idx_users_email", Kind: SymbolKindType, Language: "sql", Path: "index.sql"}},
		},
		{
			name: "create function postgresql style",
			path: "function.sql",
			src: `CREATE OR REPLACE FUNCTION public.refresh_user_count()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RETURN NEW;
END;
$$;`,
			wants: []Symbol{{Name: "refresh_user_count", Kind: SymbolKindFunction, Language: "sql", Path: "function.sql", Container: "public"}},
		},
		{
			name:  "create procedure",
			path:  "procedure.sql",
			src:   `CREATE PROCEDURE archive_old_rows() BEGIN SELECT 1; END;`,
			wants: []Symbol{{Name: "archive_old_rows", Kind: SymbolKindFunction, Language: "sql", Path: "procedure.sql"}},
		},
		{
			name: "schema qualified names",
			path: "qualified.sql",
			src: `CREATE TABLE accounting.entries (
    entry_id INT64,
    amount NUMERIC(18, 2)
);
ALTER TABLE accounting.entries ADD COLUMN posted_at TIMESTAMP;
CREATE TYPE accounting.entry_status AS ENUM ('open', 'posted');`,
			wants: []Symbol{
				{Name: "entries", Kind: SymbolKindStruct, Language: "sql", Path: "qualified.sql", Container: "accounting"},
				{Name: "entry_id", Kind: SymbolKindField, Language: "sql", Path: "qualified.sql", Container: "entries"},
				{Name: "amount", Kind: SymbolKindField, Language: "sql", Path: "qualified.sql", Container: "entries"},
				{Name: "entries", Kind: SymbolKindStruct, Language: "sql", Path: "qualified.sql", Container: "accounting"},
				{Name: "entry_status", Kind: SymbolKindType, Language: "sql", Path: "qualified.sql", Container: "accounting"},
			},
		},
		{
			name: "if not exists",
			path: "if-not-exists.sql",
			src:  `CREATE TEMPORARY TABLE IF NOT EXISTS cache_items (cache_key STRING(MAX), value BYTES(MAX));`,
			wants: []Symbol{
				{Name: "cache_items", Kind: SymbolKindStruct, Language: "sql", Path: "if-not-exists.sql"},
				{Name: "cache_key", Kind: SymbolKindField, Language: "sql", Path: "if-not-exists.sql", Container: "cache_items"},
				{Name: "value", Kind: SymbolKindField, Language: "sql", Path: "if-not-exists.sql", Container: "cache_items"},
			},
		},
		{
			name: "case insensitivity",
			path: "case.sql",
			src:  `cReAtE tAbLe MixedCase (ID INT64, DisplayName STRING(255)); CREATE TRIGGER sync_users BEFORE INSERT ON MixedCase FOR EACH ROW SET NEW.DisplayName = UPPER(NEW.DisplayName);`,
			wants: []Symbol{
				{Name: "MixedCase", Kind: SymbolKindStruct, Language: "sql", Path: "case.sql"},
				{Name: "ID", Kind: SymbolKindField, Language: "sql", Path: "case.sql", Container: "MixedCase"},
				{Name: "DisplayName", Kind: SymbolKindField, Language: "sql", Path: "case.sql", Container: "MixedCase"},
				{Name: "sync_users", Kind: SymbolKindFunction, Language: "sql", Path: "case.sql"},
			},
		},
		{
			name: "spanner ddl with interleave",
			path: "spanner.sql",
			src: `CREATE TABLE Singers (
    SingerId INT64 NOT NULL,
    FirstName STRING(1024),
    LastName STRING(1024),
) PRIMARY KEY (SingerId);

CREATE TABLE Albums (
    SingerId INT64 NOT NULL,
    AlbumId INT64 NOT NULL,
    AlbumTitle STRING(MAX),
) PRIMARY KEY (SingerId, AlbumId),
INTERLEAVE IN PARENT Singers ON DELETE CASCADE;`,
			wants: []Symbol{
				{Name: "Singers", Kind: SymbolKindStruct, Language: "sql", Path: "spanner.sql"},
				{Name: "SingerId", Kind: SymbolKindField, Language: "sql", Path: "spanner.sql", Container: "Singers"},
				{Name: "FirstName", Kind: SymbolKindField, Language: "sql", Path: "spanner.sql", Container: "Singers"},
				{Name: "LastName", Kind: SymbolKindField, Language: "sql", Path: "spanner.sql", Container: "Singers"},
				{Name: "Albums", Kind: SymbolKindStruct, Language: "sql", Path: "spanner.sql"},
				{Name: "SingerId", Kind: SymbolKindField, Language: "sql", Path: "spanner.sql", Container: "Albums"},
				{Name: "AlbumId", Kind: SymbolKindField, Language: "sql", Path: "spanner.sql", Container: "Albums"},
				{Name: "AlbumTitle", Kind: SymbolKindField, Language: "sql", Path: "spanner.sql", Container: "Albums"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			symbols, err := ExtractSQLSymbols(tt.path, []byte(tt.src))
			if err != nil {
				t.Fatalf("ExtractSQLSymbols() error = %v", err)
			}
			for _, want := range tt.wants {
				assertHasSymbol(t, symbols, want)
			}
			for _, symbol := range symbols {
				if symbol.Range.StartLine == 0 || symbol.Range.StartColumn == 0 {
					t.Fatalf("symbol %q has invalid range %#v", symbol.Name, symbol.Range)
				}
			}
		})
	}
}

func TestExtractSymbolsGenericSQLRoute(t *testing.T) {
	t.Parallel()

	symbols, err := ExtractSymbolsGeneric("schema.sql", []byte(`CREATE TABLE users (id INT64, email STRING(MAX));`))
	if err != nil {
		t.Fatalf("ExtractSymbolsGeneric() error = %v", err)
	}

	assertHasSymbol(t, symbols, Symbol{Name: "users", Kind: SymbolKindStruct, Language: "sql", Path: "schema.sql"})
	assertHasSymbol(t, symbols, Symbol{Name: "id", Kind: SymbolKindField, Language: "sql", Path: "schema.sql", Container: "users"})
	assertHasSymbol(t, symbols, Symbol{Name: "email", Kind: SymbolKindField, Language: "sql", Path: "schema.sql", Container: "users"})
}
