package structural

import (
	"strings"
	"testing"
)

func TestFindMatchingBrace_HandlesQuotesAndComments(t *testing.T) {
	t.Parallel()

	text := "{\n" +
		"// } in a line comment\n" +
		"const quoted = \"} still quoted\"\n" +
		"/* } in a block comment */\n" +
		"const tmpl = `} inside template`\n" +
		"if (true) { return { value: 1 } }\n" +
		"}"

	open := strings.IndexByte(text, '{')
	if got := findMatchingBrace(text, open); got != len(text)-1 {
		t.Fatalf("findMatchingBrace() = %d, want %d", got, len(text)-1)
	}
	if got := findMatchingBrace("{", 0); got != -1 {
		t.Fatalf("findMatchingBrace() on unterminated input = %d, want -1", got)
	}
}

func TestRustScannerHelpers(t *testing.T) {
	t.Parallel()

	comment := "/* outer /* inner */ done */"
	if got := rustScanBlockComment(comment, 0); got != len(comment) {
		t.Fatalf("rustScanBlockComment() = %d, want %d", got, len(comment))
	}

	quoted := "\"a\\\"b\""
	if got := rustScanQuotedLiteral(quoted, 0, '"'); got != len(quoted) {
		t.Fatalf("rustScanQuotedLiteral() = %d, want %d", got, len(quoted))
	}
	if got := rustScanQuotedLiteral("\"unterminated\n", 0, '"'); got != 0 {
		t.Fatalf("rustScanQuotedLiteral() for newline = %d, want 0", got)
	}
	if got := rustScanQuotedLiteral("\"unterminated", 0, '"'); got != 0 {
		t.Fatalf("rustScanQuotedLiteral() for EOF = %d, want 0", got)
	}

	if got := rustAdvance("abc;def", 0, len("abc;def")); got != 4 {
		t.Fatalf("rustAdvance() = %d, want 4", got)
	}
	if got := rustAdvance("abcdef", 0, len("abcdef")); got != len("abcdef") {
		t.Fatalf("rustAdvance() without delimiter = %d, want %d", got, len("abcdef"))
	}

	commaText := "Foo<Bar, Baz>, Qux"
	if got := rustAdvanceToNextComma(commaText, 0, len(commaText)); got != 14 {
		t.Fatalf("rustAdvanceToNextComma() = %d, want %d", got, 14)
	}
}

func TestRustMaskSource_BlanksCommentsAndStrings(t *testing.T) {
	t.Parallel()

	text := "// line comment\n" +
		"let value = \"quoted\";\n" +
		"/* block comment */\n" +
		"let raw = r#\"raw text\"#;\n" +
		"let bytes = b\"bytes\";\n"

	masked := rustMaskSource(text)
	if strings.Contains(masked, "line comment") {
		t.Fatalf("rustMaskSource() preserved line comment: %q", masked)
	}
	if strings.Contains(masked, "quoted") {
		t.Fatalf("rustMaskSource() preserved quoted string: %q", masked)
	}
	if strings.Contains(masked, "block comment") {
		t.Fatalf("rustMaskSource() preserved block comment: %q", masked)
	}
	if strings.Contains(masked, "raw text") {
		t.Fatalf("rustMaskSource() preserved raw string: %q", masked)
	}
	if strings.Contains(masked, "\"bytes\"") {
		t.Fatalf("rustMaskSource() preserved byte string literal: %q", masked)
	}
}
