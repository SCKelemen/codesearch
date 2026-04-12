package linguist

import "strings"

type Language struct {
	Name       string
	Color      string
	Type       string
	Extensions []string
	Aliases    []string
	Group      string
}

var Languages = map[string]*Language{}

func LookupByExtension(ext string) *Language {
	needle := normalizeExtension(ext)
	if needle == "" {
		return nil
	}
	for _, language := range Languages {
		for _, candidate := range language.Extensions {
			if normalizeExtension(candidate) == needle {
				return language
			}
		}
	}
	return nil
}

func LookupByName(name string) *Language {
	needle := strings.TrimSpace(strings.ToLower(name))
	if needle == "" {
		return nil
	}
	if language, ok := Languages[needle]; ok {
		return language
	}
	for key, language := range Languages {
		if strings.ToLower(key) == needle {
			return language
		}
		if strings.ToLower(language.Name) == needle {
			return language
		}
		for _, alias := range language.Aliases {
			if strings.ToLower(alias) == needle {
				return language
			}
		}
	}
	return nil
}

func normalizeExtension(ext string) string {
	trimmed := strings.TrimSpace(ext)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, ".") {
		trimmed = "." + trimmed
	}
	return strings.ToLower(trimmed)
}
