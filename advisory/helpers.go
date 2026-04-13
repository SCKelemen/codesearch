package advisory

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func readJSONResponse(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	return payload, nil
}

func preferredDescription(descriptions []nvdDesc) string {
	for _, description := range descriptions {
		if strings.EqualFold(description.Lang, "en") {
			return description.Value
		}
	}
	if len(descriptions) > 0 {
		return descriptions[0].Value
	}
	return ""
}

func parseFeedTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse time %q: unsupported format", value)
}

func formatRange(introduced, fixed, lastAffected string) string {
	parts := make([]string, 0, 3)
	if introduced != "" {
		parts = append(parts, ">="+introduced)
	}
	if fixed != "" {
		parts = append(parts, "<"+fixed)
	}
	if lastAffected != "" {
		parts = append(parts, "<="+lastAffected)
	}
	return strings.Join(parts, ", ")
}

func ecosystemFromString(value string) Ecosystem {
	switch strings.ToLower(value) {
	case "npm":
		return EcosystemNPM
	case "pypi", "pip":
		return EcosystemPyPI
	case "go", "golang":
		return EcosystemGo
	case "cargo", "rust":
		return EcosystemCargo
	case "rubygems":
		return EcosystemRubyGems
	case "maven":
		return EcosystemMaven
	case "nuget":
		return EcosystemNuGet
	default:
		return Ecosystem(strings.ToLower(value))
	}
}

func parseSeverityScore(score string) (float64, error) {
	if value, err := strconv.ParseFloat(score, 64); err == nil {
		return value, nil
	}
	marker := "CVSS:3.1/"
	if strings.HasPrefix(score, marker) {
		segments := strings.Split(score[len(marker):], "/")
		for _, segment := range segments {
			if strings.HasPrefix(segment, "SCORE:") {
				return strconv.ParseFloat(strings.TrimPrefix(segment, "SCORE:"), 64)
			}
		}
	}
	return 0, fmt.Errorf("unsupported severity score %q", score)
}
