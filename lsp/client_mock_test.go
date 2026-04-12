package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockMessage struct {
	ContentLength int
	Payload       []byte
	Request       jsonrpcRequest
}

type mockLSPServer struct {
	in        *io.PipeReader
	out       *io.PipeWriter
	messages  chan mockMessage
	responses map[string]json.RawMessage
	writeMu   sync.Mutex
}

func newMockClientServer(t *testing.T) (*Client, *mockLSPServer) {
	t.Helper()

	clientToServerReader, clientToServerWriter := io.Pipe()
	serverToClientReader, serverToClientWriter := io.Pipe()

	client := &Client{
		stdin:  clientToServerWriter,
		stdout: bufio.NewReader(serverToClientReader),
		done:   make(chan struct{}),
	}
	server := &mockLSPServer{
		in:        clientToServerReader,
		out:       serverToClientWriter,
		messages:  make(chan mockMessage, 16),
		responses: make(map[string]json.RawMessage),
	}

	go client.readLoop()
	go server.loop(t)

	t.Cleanup(func() {
		_ = clientToServerWriter.Close()
		_ = clientToServerReader.Close()
		_ = serverToClientWriter.Close()
		_ = serverToClientReader.Close()
		client.closeDone()
	})

	return client, server
}

func (s *mockLSPServer) loop(t *testing.T) {
	t.Helper()

	reader := bufio.NewReader(s.in)
	for {
		msg, err := readMockMessage(reader)
		if err != nil {
			return
		}
		if err := json.Unmarshal(msg.Payload, &msg.Request); err != nil {
			t.Errorf("json.Unmarshal() error = %v", err)
			continue
		}
		s.messages <- msg

		if msg.Request.ID != 0 {
			result, ok := s.responses[msg.Request.Method]
			if !ok {
				result = json.RawMessage("null")
			}
			s.writeMessage(t, jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      msg.Request.ID,
				Result:  result,
			})
		}

		if msg.Request.Method == "exit" {
			_ = s.out.Close()
			return
		}
	}
}

func (s *mockLSPServer) setResult(t *testing.T, method string, result any) {
	t.Helper()

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	s.responses[method] = data
}

func (s *mockLSPServer) writeMessage(t *testing.T, payload any) {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if _, err := fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		t.Fatalf("fmt.Fprintf() error = %v", err)
	}
	if _, err := s.out.Write(data); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func (s *mockLSPServer) sendNotification(t *testing.T, method string, params any) {
	t.Helper()

	paramsRaw, err := marshalParams(params)
	if err != nil {
		t.Fatalf("marshalParams() error = %v", err)
	}
	s.writeMessage(t, jsonrpcResponse{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsRaw,
	})
}

func (s *mockLSPServer) nextMessage(t *testing.T) mockMessage {
	t.Helper()

	select {
	case msg := <-s.messages:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mock LSP message")
		return mockMessage{}
	}
}

func readMockMessage(reader *bufio.Reader) (mockMessage, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return mockMessage{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(parts[0]), "Content-Length") {
			continue
		}
		contentLength, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return mockMessage{}, err
		}
	}
	if contentLength < 0 {
		return mockMessage{}, fmt.Errorf("missing Content-Length header")
	}

	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return mockMessage{}, err
	}
	return mockMessage{ContentLength: contentLength, Payload: payload}, nil
}

func TestNewClient_CommandNotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client, err := NewClient(ctx, t.TempDir(), []string{"codesearch-missing-lsp-command"})
	if err == nil {
		_ = client.Close()
		t.Fatal("NewClient() error = nil, want command-not-found error")
	}
}

func TestSendNotification_Format(t *testing.T) {
	var buf bytes.Buffer
	client := &Client{
		stdin: nopWriteCloser{Writer: &buf},
		done:  make(chan struct{}),
	}

	if err := client.sendNotification("initialized", map[string]any{"ready": true}); err != nil {
		t.Fatalf("sendNotification() error = %v", err)
	}

	raw := buf.String()
	parts := strings.SplitN(raw, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("sendNotification() wrote %q, want Content-Length framing", raw)
	}
	if !strings.HasPrefix(parts[0], "Content-Length: ") {
		t.Fatalf("headers = %q, want Content-Length header", parts[0])
	}

	var msg jsonrpcRequest
	if err := json.Unmarshal([]byte(parts[1]), &msg); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if msg.JSONRPC != "2.0" {
		t.Fatalf("jsonrpc = %q, want 2.0", msg.JSONRPC)
	}
	if msg.Method != "initialized" {
		t.Fatalf("method = %q, want initialized", msg.Method)
	}
	if msg.ID != 0 {
		t.Fatalf("id = %d, want notification without request ID", msg.ID)
	}

	var params map[string]bool
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("json.Unmarshal(params) error = %v", err)
	}
	if !params["ready"] {
		t.Fatalf("params = %#v, want ready=true", params)
	}
}

func TestSendRequest_Format(t *testing.T) {
	client, server := newMockClientServer(t)
	server.setResult(t, "workspace/symbol", []string{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := client.sendRequestContext(ctx, "workspace/symbol", map[string]any{"query": "fmt"}); err != nil {
		t.Fatalf("sendRequestContext() error = %v", err)
	}

	msg := server.nextMessage(t)
	if msg.ContentLength != len(msg.Payload) {
		t.Fatalf("Content-Length = %d, want %d", msg.ContentLength, len(msg.Payload))
	}
	if msg.Request.JSONRPC != "2.0" {
		t.Fatalf("jsonrpc = %q, want 2.0", msg.Request.JSONRPC)
	}
	if msg.Request.Method != "workspace/symbol" {
		t.Fatalf("method = %q, want workspace/symbol", msg.Request.Method)
	}
	if msg.Request.ID == 0 {
		t.Fatal("request ID = 0, want non-zero request ID")
	}

	var params map[string]string
	if err := json.Unmarshal(msg.Request.Params, &params); err != nil {
		t.Fatalf("json.Unmarshal(params) error = %v", err)
	}
	if params["query"] != "fmt" {
		t.Fatalf("params = %#v, want query=fmt", params)
	}
}

func TestDiagnostics_Empty(t *testing.T) {
	client := &Client{}
	if diagnostics := client.Diagnostics("file:///missing.go"); len(diagnostics) != 0 {
		t.Fatalf("Diagnostics() len = %d, want 0", len(diagnostics))
	}
}

func TestClose_Idempotent(t *testing.T) {
	client, server := newMockClientServer(t)
	server.setResult(t, "shutdown", nil)

	if err := client.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	first := server.nextMessage(t)
	second := server.nextMessage(t)
	methods := []string{first.Request.Method, second.Request.Method}
	if methods[0] != "shutdown" || methods[1] != "exit" {
		t.Fatalf("Close() methods = %v, want [shutdown exit]", methods)
	}
}

func TestFileURI_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific path handling")
	}

	path := `C:\Users\sam\project\main.go`
	uri := FileURI(path)
	if !strings.HasPrefix(uri, "file:///C:/Users/sam/project/") {
		t.Fatalf("FileURI() = %q, want windows file URI", uri)
	}
	if got := URIToPath(uri); got != path {
		t.Fatalf("URIToPath(FileURI()) = %q, want %q", got, path)
	}
}

func TestURIToPath_InvalidScheme(t *testing.T) {
	if got := URIToPath("https://example.com/main.go"); got != "" {
		t.Fatalf("URIToPath() = %q, want empty string", got)
	}
}

func TestOpenFile_MockServer(t *testing.T) {
	client, server := newMockClientServer(t)
	uri := FileURI("example.go")

	if err := client.OpenFile(context.Background(), uri, "package main\n"); err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}

	msg := server.nextMessage(t)
	if msg.Request.Method != "textDocument/didOpen" {
		t.Fatalf("method = %q, want textDocument/didOpen", msg.Request.Method)
	}

	var params struct {
		TextDocument struct {
			URI        string `json:"uri"`
			LanguageID string `json:"languageId"`
			Version    int    `json:"version"`
			Text       string `json:"text"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(msg.Request.Params, &params); err != nil {
		t.Fatalf("json.Unmarshal(params) error = %v", err)
	}
	if params.TextDocument.URI != uri {
		t.Fatalf("uri = %q, want %q", params.TextDocument.URI, uri)
	}
	if params.TextDocument.LanguageID != "go" {
		t.Fatalf("languageId = %q, want go", params.TextDocument.LanguageID)
	}
	if params.TextDocument.Version != 1 {
		t.Fatalf("version = %d, want 1", params.TextDocument.Version)
	}
	if params.TextDocument.Text != "package main\n" {
		t.Fatalf("text = %q, want file contents", params.TextDocument.Text)
	}
}

func TestDocumentSymbols_MockServer(t *testing.T) {
	client, server := newMockClientServer(t)
	uri := FileURI("symbols.go")
	server.setResult(t, "textDocument/documentSymbol", []map[string]any{{
		"name": "Example",
		"kind": 12,
		"range": map[string]any{
			"start": map[string]any{"line": 1, "character": 0},
			"end":   map[string]any{"line": 3, "character": 1},
		},
		"selectionRange": map[string]any{
			"start": map[string]any{"line": 1, "character": 5},
			"end":   map[string]any{"line": 1, "character": 12},
		},
	}})

	symbols, err := client.DocumentSymbols(context.Background(), uri)
	if err != nil {
		t.Fatalf("DocumentSymbols() error = %v", err)
	}
	if len(symbols) != 1 {
		t.Fatalf("len(symbols) = %d, want 1", len(symbols))
	}
	if symbols[0].Name != "Example" {
		t.Fatalf("symbol name = %q, want Example", symbols[0].Name)
	}

	msg := server.nextMessage(t)
	if msg.Request.Method != "textDocument/documentSymbol" {
		t.Fatalf("method = %q, want textDocument/documentSymbol", msg.Request.Method)
	}
}

func TestHover_MockServer(t *testing.T) {
	client, server := newMockClientServer(t)
	uri := FileURI("hover.go")
	server.setResult(t, "textDocument/hover", map[string]any{
		"contents": map[string]any{
			"kind":  "markdown",
			"value": "hover text",
		},
	})

	hover, err := client.Hover(context.Background(), uri, 4, 2)
	if err != nil {
		t.Fatalf("Hover() error = %v", err)
	}
	if hover == nil || hover.Contents != "hover text" {
		t.Fatalf("Hover() = %#v, want hover text", hover)
	}

	msg := server.nextMessage(t)
	if msg.Request.Method != "textDocument/hover" {
		t.Fatalf("method = %q, want textDocument/hover", msg.Request.Method)
	}
}

func TestDefinition_MockServer(t *testing.T) {
	client, server := newMockClientServer(t)
	uri := FileURI("definition.go")
	server.setResult(t, "textDocument/definition", []map[string]any{{
		"uri": uri,
		"range": map[string]any{
			"start": map[string]any{"line": 7, "character": 1},
			"end":   map[string]any{"line": 7, "character": 9},
		},
	}})

	locations, err := client.Definition(context.Background(), uri, 1, 1)
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	if len(locations) != 1 {
		t.Fatalf("len(locations) = %d, want 1", len(locations))
	}
	if locations[0].URI != uri {
		t.Fatalf("location URI = %q, want %q", locations[0].URI, uri)
	}

	msg := server.nextMessage(t)
	if msg.Request.Method != "textDocument/definition" {
		t.Fatalf("method = %q, want textDocument/definition", msg.Request.Method)
	}
}

func TestReferences_MockServer(t *testing.T) {
	client, server := newMockClientServer(t)
	uri := FileURI("references.go")
	server.setResult(t, "textDocument/references", []map[string]any{{
		"uri": uri,
		"range": map[string]any{
			"start": map[string]any{"line": 2, "character": 3},
			"end":   map[string]any{"line": 2, "character": 7},
		},
	}})

	locations, err := client.References(context.Background(), uri, 2, 3, true)
	if err != nil {
		t.Fatalf("References() error = %v", err)
	}
	if len(locations) != 1 {
		t.Fatalf("len(locations) = %d, want 1", len(locations))
	}

	msg := server.nextMessage(t)
	if msg.Request.Method != "textDocument/references" {
		t.Fatalf("method = %q, want textDocument/references", msg.Request.Method)
	}
}

func TestReadLoop_Notification(t *testing.T) {
	client, server := newMockClientServer(t)
	uri := FileURI("diagnostics.go")
	want := []Diagnostic{{
		Range: Range{
			Start: Position{Line: 1, Character: 0},
			End:   Position{Line: 1, Character: 3},
		},
		Message:  "problem",
		Severity: 1,
	}}

	server.sendNotification(t, "textDocument/publishDiagnostics", map[string]any{
		"uri":         uri,
		"diagnostics": want,
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got := client.Diagnostics(uri)
		if len(got) == 1 && got[0].Message == "problem" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("Diagnostics(%q) = %#v, want %#v", uri, client.Diagnostics(uri), want)
}
