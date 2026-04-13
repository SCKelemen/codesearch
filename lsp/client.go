package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var errClientClosed = errors.New("lsp client closed")

// Client communicates with a language server over stdio using JSON-RPC 2.0.
type Client struct {
	id          ServerID
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      *bufio.Reader
	rootURI     string
	nextID      atomic.Int64
	writeMu     sync.Mutex
	pending     sync.Map
	diagnostics sync.Map
	done        chan struct{}
	closeOnce   sync.Once
}

// Position represents a zero-based line and character position in a text document.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range represents a span in a text document between two positions.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location identifies a document URI and range.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// Symbol represents a document symbol returned by textDocument/documentSymbol.
type Symbol struct {
	Name           string   `json:"name"`
	Kind           int      `json:"kind"`
	Range          Range    `json:"range"`
	SelectionRange Range    `json:"selectionRange"`
	Children       []Symbol `json:"children,omitempty"`
}

// HoverResult contains hover information for a symbol.
type HoverResult struct {
	Contents string
}

// Diagnostic represents a compiler or language-server diagnostic.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"`
	Code     any    `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *jsonrpcError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("json-rpc error %d: %s", e.Code, e.Message)
}

// NewClient starts a language server subprocess and initializes an LSP connection.
func NewClient(ctx context.Context, workDir string, command []string) (*Client, error) {
	if len(command) == 0 {
		return nil, errors.New("lsp command is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = workDir
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}

	client := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		rootURI: FileURI(workDir),
		done:    make(chan struct{}),
	}
	go client.readLoop()

	if err := client.initialize(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}

	var closeErr error
	c.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if _, err := c.sendRequestContext(ctx, "shutdown", nil); err != nil && !errors.Is(err, errClientClosed) {
			closeErr = err
		}
		if err := c.sendNotification("exit", nil); err != nil && closeErr == nil && !errors.Is(err, io.ErrClosedPipe) {
			closeErr = err
		}

		c.closeDone()
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		if c.cmd != nil && c.cmd.Process != nil {
			if err := c.cmd.Process.Kill(); err != nil && closeErr == nil && !errors.Is(err, os.ErrProcessDone) {
				closeErr = err
			}
		}
		if c.cmd != nil {
			_ = c.cmd.Wait()
		}
	})

	return closeErr
}

func (c *Client) OpenFile(ctx context.Context, uri string, content string) error {
	if err := ctxErr(ctx); err != nil {
		return fmt.Errorf("open file %q: %w", uri, err)
	}

	params := map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": languageID(uri),
			"version":    1,
			"text":       content,
		},
	}
	return c.sendNotification("textDocument/didOpen", params)
}

func (c *Client) DocumentSymbols(ctx context.Context, uri string) ([]Symbol, error) {
	raw, err := c.request(ctx, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
	if err != nil {
		return nil, err
	}
	if isJSONNull(raw) {
		return nil, nil
	}

	var symbols []Symbol
	if err := json.Unmarshal(raw, &symbols); err == nil {
		return symbols, nil
	}

	var symbolInfos []struct {
		Name     string   `json:"name"`
		Kind     int      `json:"kind"`
		Location Location `json:"location"`
	}
	if err := json.Unmarshal(raw, &symbolInfos); err != nil {
		return nil, err
	}

	symbols = make([]Symbol, 0, len(symbolInfos))
	for _, info := range symbolInfos {
		symbols = append(symbols, Symbol{
			Name:           info.Name,
			Kind:           info.Kind,
			Range:          info.Location.Range,
			SelectionRange: info.Location.Range,
		})
	}
	return symbols, nil
}

func (c *Client) Hover(ctx context.Context, uri string, line, col int) (*HoverResult, error) {
	raw, err := c.request(ctx, "textDocument/hover", textDocumentPositionParams(uri, line, col))
	if err != nil {
		return nil, err
	}
	if isJSONNull(raw) {
		return nil, nil
	}

	return &HoverResult{Contents: extractHoverText(raw)}, nil
}

func (c *Client) Definition(ctx context.Context, uri string, line, col int) ([]Location, error) {
	raw, err := c.request(ctx, "textDocument/definition", textDocumentPositionParams(uri, line, col))
	if err != nil {
		return nil, err
	}
	return decodeLocations(raw)
}

func (c *Client) References(ctx context.Context, uri string, line, col int, includeDecl bool) ([]Location, error) {
	params := textDocumentPositionParams(uri, line, col)
	params["context"] = map[string]any{"includeDeclaration": includeDecl}

	raw, err := c.request(ctx, "textDocument/references", params)
	if err != nil {
		return nil, err
	}
	return decodeLocations(raw)
}

func (c *Client) Diagnostics(uri string) []Diagnostic {
	value, ok := c.diagnostics.Load(uri)
	if !ok {
		return nil
	}
	diagnostics, ok := value.([]Diagnostic)
	if !ok {
		return nil
	}
	out := make([]Diagnostic, len(diagnostics))
	copy(out, diagnostics)
	return out
}

func (c *Client) sendRequest(method string, params any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.sendRequestContext(ctx, method, params)
}

func (c *Client) sendRequestContext(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c == nil {
		return nil, errClientClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-c.done:
		return nil, errClientClosed
	default:
	}

	paramsRaw, err := marshalParams(params)
	if err != nil {
		return nil, err
	}

	id := c.nextID.Add(1)
	respCh := make(chan jsonrpcResponse, 1)
	c.pending.Store(id, respCh)
	defer c.pending.Delete(id)

	if err := c.writeMessage(jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsRaw,
	}); err != nil {
		return nil, err
	}

	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, errClientClosed
	}
}

func (c *Client) sendNotification(method string, params any) error {
	if c == nil {
		return errClientClosed
	}
	select {
	case <-c.done:
		return errClientClosed
	default:
	}

	paramsRaw, err := marshalParams(params)
	if err != nil {
		return fmt.Errorf("marshal notification params for %q: %w", method, err)
	}

	return c.writeMessage(jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsRaw,
	})
}

func (c *Client) readLoop() {
	defer c.closeDone()
	defer c.failPending(errClientClosed)

	for {
		payload, err := c.readMessage()
		if err != nil {
			return
		}

		var msg jsonrpcResponse
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}

		if msg.Method != "" {
			c.handleNotification(msg.Method, msg.Params)
			continue
		}

		value, ok := c.pending.Load(msg.ID)
		if !ok {
			continue
		}
		respCh, ok := value.(chan jsonrpcResponse)
		if !ok {
			continue
		}
		select {
		case respCh <- msg:
		default:
		}
	}
}

func (c *Client) initialize(ctx context.Context) error {
	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   c.rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"hover": map[string]any{
					"dynamicRegistration": false,
				},
				"definition": map[string]any{
					"dynamicRegistration": false,
				},
				"references": map[string]any{
					"dynamicRegistration": false,
				},
				"documentSymbol": map[string]any{
					"dynamicRegistration": false,
				},
				"publishDiagnostics": map[string]any{
					"relatedInformation": true,
				},
			},
		},
	}

	if _, err := c.sendRequestContext(ctx, "initialize", params); err != nil {
		return fmt.Errorf("initialize language server: %w", err)
	}
	return c.sendNotification("initialized", map[string]any{})
}

// FileURI converts a filesystem path to a file:// URI.
func FileURI(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
}

// URIToPath converts a file:// URI back to a filesystem path.
func URIToPath(uri string) string {
	if uri == "" {
		return ""
	}
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}

	path, err := url.PathUnescape(u.EscapedPath())
	if err != nil {
		path = u.Path
	}
	if u.Host != "" && u.Host != "localhost" {
		path = "//" + u.Host + path
	}
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

func extractHoverText(raw json.RawMessage) string {
	if isJSONNull(raw) {
		return ""
	}

	var hover struct {
		Contents json.RawMessage `json:"contents"`
	}
	if len(raw) > 0 && raw[0] == '{' && json.Unmarshal(raw, &hover) == nil && len(hover.Contents) > 0 {
		return extractHoverText(hover.Contents)
	}

	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}

	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) == nil {
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if text := extractHoverText(part); text != "" {
				out = append(out, text)
			}
		}
		return strings.Join(out, "\n\n")
	}

	var markup struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &markup) == nil && markup.Value != "" {
		return markup.Value
	}

	return ""
}

func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return c.sendRequestContext(ctx, method, params)
}

func (c *Client) writeMessage(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal JSON-RPC message: %w", err)
	}
	return c.writePayload(payload)
}

func (c *Client) writePayload(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return fmt.Errorf("write JSON-RPC header: %w", err)
	}
	_, err := c.stdin.Write(payload)
	if err != nil {
		return fmt.Errorf("write JSON-RPC payload: %w", err)
	}
	return nil
}

func (c *Client) readMessage() ([]byte, error) {
	contentLength := -1
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return nil, err
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
			return nil, err
		}
	}
	if contentLength < 0 {
		return nil, errors.New("missing Content-Length header")
	}

	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(c.stdout, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) handleNotification(method string, params json.RawMessage) {
	if method != "textDocument/publishDiagnostics" {
		return
	}

	var payload struct {
		URI         string       `json:"uri"`
		Diagnostics []Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(params, &payload); err != nil {
		return
	}
	out := make([]Diagnostic, len(payload.Diagnostics))
	copy(out, payload.Diagnostics)
	c.diagnostics.Store(payload.URI, out)
}

func (c *Client) failPending(err error) {
	c.pending.Range(func(_, value any) bool {
		respCh, ok := value.(chan jsonrpcResponse)
		if ok {
			select {
			case respCh <- jsonrpcResponse{Error: &jsonrpcError{Code: -32000, Message: err.Error()}}:
			default:
			}
		}
		return true
	})
}

func (c *Client) closeDone() {
	if c == nil || c.done == nil {
		return
	}
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

func textDocumentPositionParams(uri string, line, col int) map[string]any {
	return map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position": map[string]any{
			"line":      line,
			"character": col,
		},
	}
}

func decodeLocations(raw json.RawMessage) ([]Location, error) {
	if isJSONNull(raw) {
		return nil, nil
	}

	var locations []Location
	if err := json.Unmarshal(raw, &locations); err == nil {
		return locations, nil
	}

	var location Location
	if err := json.Unmarshal(raw, &location); err == nil && location.URI != "" {
		return []Location{location}, nil
	}

	var links []struct {
		TargetURI   string `json:"targetUri"`
		TargetRange Range  `json:"targetRange"`
	}
	if err := json.Unmarshal(raw, &links); err != nil {
		return nil, err
	}
	locations = make([]Location, 0, len(links))
	for _, link := range links {
		locations = append(locations, Location{URI: link.TargetURI, Range: link.TargetRange})
	}
	return locations, nil
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func isJSONNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}

func languageID(uri string) string {
	ext := strings.ToLower(filepath.Ext(URIToPath(uri)))
	switch ext {
	case ".go":
		return "go"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".json":
		return "json"
	default:
		return strings.TrimPrefix(ext, ".")
	}
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
