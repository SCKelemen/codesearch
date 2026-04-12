package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error {
	return nil
}

func TestFileURI(t *testing.T) {
	uri := FileURI("testdata/file with spaces.go")
	if !strings.HasPrefix(uri, "file://") {
		t.Fatalf("FileURI() = %q, want file:// prefix", uri)
	}
	want := URIToPath(FileURI("testdata/file with spaces.go"))
	if got := URIToPath(uri); got != want {
		t.Fatalf("round trip path = %q, want %q", got, want)
	}
}

func TestURIToPath(t *testing.T) {
	got := URIToPath("file:///tmp/example%20file.go")
	want := "/tmp/example file.go"
	if got != want {
		t.Fatalf("URIToPath() = %q, want %q", got, want)
	}
}

func TestExtractHoverText(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{
			name: "string",
			raw:  json.RawMessage(`"hello"`),
			want: "hello",
		},
		{
			name: "array",
			raw:  json.RawMessage(`[{"language":"go","value":"fmt.Println"},"extra"]`),
			want: "fmt.Println\n\nextra",
		},
		{
			name: "markup",
			raw:  json.RawMessage(`{"contents":{"kind":"markdown","value":"**bold**"}}`),
			want: "**bold**",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractHoverText(tt.raw); got != tt.want {
				t.Fatalf("extractHoverText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContentLengthFraming(t *testing.T) {
	var buf bytes.Buffer
	client := &Client{stdin: nopWriteCloser{Writer: &buf}}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialized",
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := client.writeMessage(payload); err != nil {
		t.Fatalf("writeMessage() error = %v", err)
	}
	want := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payloadBytes), payloadBytes)
	if got := buf.String(); got != want {
		t.Fatalf("writeMessage() = %q, want %q", got, want)
	}
}
