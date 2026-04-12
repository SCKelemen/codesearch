package embedding

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Chunk contains an embeddable slice of a source file.
type Chunk struct {
	Path      string
	StartLine int
	EndLine   int
	Content   string
	Language  string
}

// ChunkStrategy splits file contents into embeddable chunks.
type ChunkStrategy interface {
	Chunk(path string, content []byte) ([]Chunk, error)
}

// FileChunker emits a single chunk per file.
type FileChunker struct{}

// Chunk returns one chunk for the entire file.
func (FileChunker) Chunk(path string, content []byte) ([]Chunk, error) {
	lines := splitLines(content)
	if len(lines) == 0 {
		return nil, nil
	}

	return []Chunk{makeChunk(path, detectLanguage(path), lines, 1, len(lines))}, nil
}

// FixedWindowChunker emits overlapping line windows.
type FixedWindowChunker struct {
	Size    int
	Overlap int
}

// Chunk returns sliding line windows with overlap.
func (c FixedWindowChunker) Chunk(path string, content []byte) ([]Chunk, error) {
	if c.Size <= 0 {
		return nil, fmt.Errorf("fixed window size must be greater than zero")
	}
	if c.Overlap < 0 {
		return nil, fmt.Errorf("fixed window overlap must be non-negative")
	}
	if c.Overlap >= c.Size {
		return nil, fmt.Errorf("fixed window overlap must be smaller than size")
	}

	lines := splitLines(content)
	if len(lines) == 0 {
		return nil, nil
	}

	language := detectLanguage(path)
	step := c.Size - c.Overlap
	chunks := make([]Chunk, 0, (len(lines)+step-1)/step)
	for start := 1; start <= len(lines); start += step {
		end := start + c.Size - 1
		if end > len(lines) {
			end = len(lines)
		}
		chunks = append(chunks, makeChunk(path, language, lines, start, end))
		if end == len(lines) {
			break
		}
	}

	return chunks, nil
}

// FunctionChunker splits files on simple function and class boundaries.
type FunctionChunker struct{}

var functionBoundaryPattern = regexp.MustCompile(`^\s*(func\s+(?:\([^)]*\)\s*)?[A-Za-z_][A-Za-z0-9_]*\s*\(|def\s+[A-Za-z_][A-Za-z0-9_]*\s*\(|class\s+[A-Za-z_][A-Za-z0-9_]*(?:\(|:)|(?:export\s+)?(?:async\s+)?function\s+[A-Za-z_][A-Za-z0-9_]*\s*\(|(?:export\s+)?class\s+[A-Za-z_][A-Za-z0-9_]*|(?:export\s+)?(?:const|let|var)\s+[A-Za-z_][A-Za-z0-9_]*\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_][A-Za-z0-9_]*)\s*=>)`)

// Chunk splits file contents on matching boundaries.
func (FunctionChunker) Chunk(path string, content []byte) ([]Chunk, error) {
	lines := splitLines(content)
	if len(lines) == 0 {
		return nil, nil
	}

	var starts []int
	for i, line := range lines {
		if functionBoundaryPattern.MatchString(line) {
			starts = append(starts, i+1)
		}
	}
	if len(starts) == 0 {
		return FileChunker{}.Chunk(path, content)
	}

	language := detectLanguage(path)
	chunks := make([]Chunk, 0, len(starts)+1)
	if starts[0] > 1 {
		preamble := makeChunk(path, language, lines, 1, starts[0]-1)
		if strings.TrimSpace(preamble.Content) != "" {
			chunks = append(chunks, preamble)
		}
	}

	for i, start := range starts {
		end := len(lines)
		if i+1 < len(starts) {
			end = starts[i+1] - 1
		}
		chunk := makeChunk(path, language, lines, start, end)
		if strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

func splitLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}

	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func makeChunk(path, language string, lines []string, startLine, endLine int) Chunk {
	return Chunk{
		Path:      path,
		StartLine: startLine,
		EndLine:   endLine,
		Content:   strings.Join(lines[startLine-1:endLine], "\n"),
		Language:  language,
	}
}

func detectLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".rs":
		return "rust"
	case ".c", ".h":
		return "c"
	case ".cc", ".cpp", ".cxx", ".hpp":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".m", ".mm":
		return "objective-c"
	default:
		return strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	}
}
