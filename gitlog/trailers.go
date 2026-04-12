package gitlog

import (
	"strings"
)

// Author represents a git commit author with name and email.
type Author struct {
	Name  string
	Email string
}

type trailer struct {
	key   string
	value string
}

// ParseCoAuthors extracts Co-Authored-By trailers from a git commit message.
func ParseCoAuthors(message string) []Author {
	trailers := parseTrailers(message)
	if len(trailers) == 0 {
		return nil
	}

	authors := make([]Author, 0, len(trailers))
	for _, trailer := range trailers {
		if !strings.EqualFold(trailer.key, "Co-Authored-By") {
			continue
		}

		name, email, ok := parseAuthor(trailer.value)
		if !ok {
			continue
		}
		authors = append(authors, Author{Name: name, Email: email})
	}
	if len(authors) == 0 {
		return nil
	}
	return authors
}

// ParseTrailers extracts all git trailers from a commit message body.
func ParseTrailers(message string) map[string][]string {
	trailers := parseTrailers(message)
	if len(trailers) == 0 {
		return nil
	}

	result := make(map[string][]string, len(trailers))
	for _, trailer := range trailers {
		result[trailer.key] = append(result[trailer.key], trailer.value)
	}
	return result
}

func parseTrailers(message string) []trailer {
	lines := strings.Split(strings.ReplaceAll(message, "\r\n", "\n"), "\n")
	end := len(lines) - 1
	for end >= 0 && strings.TrimSpace(lines[end]) == "" {
		end--
	}

	trailers := make([]trailer, 0)
	for i := end; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			break
		}
		key, value, ok := parseTrailer(line)
		if !ok {
			break
		}
		trailers = append([]trailer{{key: key, value: value}}, trailers...)
	}
	return trailers
}

func parseTrailer(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return "", "", false
	}
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return "", "", false
	}
	return key, value, true
}

func parseAuthor(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	start := strings.LastIndex(value, "<")
	end := strings.LastIndex(value, ">")
	if start == -1 || end == -1 || start >= end {
		return "", "", false
	}
	name := strings.TrimSpace(value[:start])
	email := strings.TrimSpace(value[start+1 : end])
	if name == "" || email == "" {
		return "", "", false
	}
	return name, email, true
}
