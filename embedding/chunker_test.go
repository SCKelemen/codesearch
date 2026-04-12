package embedding

import (
	"reflect"
	"strings"
	"testing"
)

const goSample = `package sample

import "fmt"

func Add(a, b int) int {
	return a + b
}

func (s Service) Run() error {
	fmt.Println("running")
	return nil
}
`

const pythonSample = `class Greeter:
	def greet(self, name):
		return f"hi {name}"


def helper(value):
	return value * 2
`

const typeScriptSample = `export function add(a: number, b: number) {
  return a + b;
}

export const double = (value: number) => {
  return value * 2;
};

export class Service {
  run() {
    return "ok";
  }
}
`

func TestFileChunker(t *testing.T) {
	chunks, err := (FileChunker{}).Chunk("example.go", []byte(goSample))
	if err != nil {
		t.Fatalf("Chunk returned error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.Path != "example.go" {
		t.Fatalf("expected path example.go, got %q", chunk.Path)
	}
	if chunk.StartLine != 1 {
		t.Fatalf("expected start line 1, got %d", chunk.StartLine)
	}
	if chunk.EndLine != 12 {
		t.Fatalf("expected end line 12, got %d", chunk.EndLine)
	}
	if chunk.Language != "go" {
		t.Fatalf("expected language go, got %q", chunk.Language)
	}
	if !strings.Contains(chunk.Content, "func Add") || !strings.Contains(chunk.Content, "func (s Service) Run") {
		t.Fatalf("chunk content did not include expected Go functions: %q", chunk.Content)
	}
}

func TestFixedWindowChunker(t *testing.T) {
	chunker := FixedWindowChunker{Size: 3, Overlap: 1}
	chunks, err := chunker.Chunk("example.ts", []byte(typeScriptSample))
	if err != nil {
		t.Fatalf("Chunk returned error: %v", err)
	}

	gotRanges := make([][2]int, len(chunks))
	for i, chunk := range chunks {
		gotRanges[i] = [2]int{chunk.StartLine, chunk.EndLine}
		if chunk.Language != "typescript" {
			t.Fatalf("expected language typescript, got %q", chunk.Language)
		}
	}

	wantRanges := [][2]int{{1, 3}, {3, 5}, {5, 7}, {7, 9}, {9, 11}, {11, 13}}
	if !reflect.DeepEqual(gotRanges, wantRanges) {
		t.Fatalf("unexpected ranges: got %v want %v", gotRanges, wantRanges)
	}
	if !strings.Contains(chunks[0].Content, "export function add") {
		t.Fatalf("expected first chunk to contain TypeScript function, got %q", chunks[0].Content)
	}
}

func TestFixedWindowChunkerRejectsInvalidConfig(t *testing.T) {
	_, err := (FixedWindowChunker{Size: 3, Overlap: 3}).Chunk("example.go", []byte(goSample))
	if err == nil {
		t.Fatal("expected overlap validation error")
	}
}

func TestFunctionChunkerGo(t *testing.T) {
	chunks, err := (FunctionChunker{}).Chunk("example.go", []byte(goSample))
	if err != nil {
		t.Fatalf("Chunk returned error: %v", err)
	}

	gotRanges := make([][2]int, len(chunks))
	for i, chunk := range chunks {
		gotRanges[i] = [2]int{chunk.StartLine, chunk.EndLine}
	}

	wantRanges := [][2]int{{1, 4}, {5, 8}, {9, 12}}
	if !reflect.DeepEqual(gotRanges, wantRanges) {
		t.Fatalf("unexpected Go chunk ranges: got %v want %v", gotRanges, wantRanges)
	}
	if !strings.Contains(chunks[1].Content, "func Add") {
		t.Fatalf("expected second chunk to contain Add function, got %q", chunks[1].Content)
	}
}

func TestFunctionChunkerPython(t *testing.T) {
	chunks, err := (FunctionChunker{}).Chunk("example.py", []byte(pythonSample))
	if err != nil {
		t.Fatalf("Chunk returned error: %v", err)
	}

	gotRanges := make([][2]int, len(chunks))
	for i, chunk := range chunks {
		gotRanges[i] = [2]int{chunk.StartLine, chunk.EndLine}
		if chunk.Language != "python" {
			t.Fatalf("expected language python, got %q", chunk.Language)
		}
	}

	wantRanges := [][2]int{{1, 1}, {2, 5}, {6, 7}}
	if !reflect.DeepEqual(gotRanges, wantRanges) {
		t.Fatalf("unexpected Python chunk ranges: got %v want %v", gotRanges, wantRanges)
	}
}

func TestFunctionChunkerTypeScript(t *testing.T) {
	chunks, err := (FunctionChunker{}).Chunk("example.ts", []byte(typeScriptSample))
	if err != nil {
		t.Fatalf("Chunk returned error: %v", err)
	}

	gotRanges := make([][2]int, len(chunks))
	for i, chunk := range chunks {
		gotRanges[i] = [2]int{chunk.StartLine, chunk.EndLine}
	}

	wantRanges := [][2]int{{1, 4}, {5, 8}, {9, 13}}
	if !reflect.DeepEqual(gotRanges, wantRanges) {
		t.Fatalf("unexpected TypeScript chunk ranges: got %v want %v", gotRanges, wantRanges)
	}
	if !strings.Contains(chunks[2].Content, "export class Service") {
		t.Fatalf("expected last chunk to contain Service class, got %q", chunks[2].Content)
	}
}
