package gitlog

import (
	"reflect"
	"testing"
)

func TestParseCoAuthors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		want    []Author
	}{
		{
			name:    "empty message returns nil",
			message: "",
			want:    nil,
		},
		{
			name:    "message with no trailers returns nil",
			message: "feat: add parser\n\nThis commit has no trailers.",
			want:    nil,
		},
		{
			name:    "single co authored by trailer",
			message: "feat: add parser\n\nCo-Authored-By: Jane Doe <jane@example.com>",
			want:    []Author{{Name: "Jane Doe", Email: "jane@example.com"}},
		},
		{
			name:    "multiple co authored by trailers",
			message: "feat: add parser\n\nCo-Authored-By: Jane Doe <jane@example.com>\nCo-Authored-By: John Smith <john@example.com>",
			want: []Author{
				{Name: "Jane Doe", Email: "jane@example.com"},
				{Name: "John Smith", Email: "john@example.com"},
			},
		},
		{
			name:    "case insensitive matching",
			message: "feat: add parser\n\nCo-authored-by: Jane Doe <jane@example.com>\nCO-AUTHORED-BY: John Smith <john@example.com>",
			want: []Author{
				{Name: "Jane Doe", Email: "jane@example.com"},
				{Name: "John Smith", Email: "john@example.com"},
			},
		},
		{
			name:    "malformed trailer no angle brackets is skipped",
			message: "feat: add parser\n\nCo-Authored-By: Jane Doe jane@example.com\nCo-Authored-By: John Smith <john@example.com>",
			want:    []Author{{Name: "John Smith", Email: "john@example.com"}},
		},
		{
			name:    "mixed trailers signed off by and co authored by",
			message: "feat: add parser\n\nSigned-off-by: Reviewer <reviewer@example.com>\nCo-Authored-By: Jane Doe <jane@example.com>",
			want:    []Author{{Name: "Jane Doe", Email: "jane@example.com"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ParseCoAuthors(tt.message)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseCoAuthors() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseTrailers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		want    map[string][]string
	}{
		{
			name:    "empty message returns nil",
			message: "",
			want:    nil,
		},
		{
			name:    "message with body but no trailers",
			message: "feat: add parser\n\nThis commit body has content but no trailers.",
			want:    nil,
		},
		{
			name:    "single trailer",
			message: "feat: add parser\n\nSigned-off-by: Jane Doe <jane@example.com>",
			want: map[string][]string{
				"Signed-off-by": {"Jane Doe <jane@example.com>"},
			},
		},
		{
			name:    "multiple values for same key",
			message: "feat: add parser\n\nCo-Authored-By: Jane Doe <jane@example.com>\nCo-Authored-By: John Smith <john@example.com>",
			want: map[string][]string{
				"Co-Authored-By": {
					"Jane Doe <jane@example.com>",
					"John Smith <john@example.com>",
				},
			},
		},
		{
			name:    "mixed trailer types",
			message: "feat: add parser\n\nReviewed-by: Reviewer <reviewer@example.com>\nSigned-off-by: Maintainer <maintainer@example.com>\nCo-Authored-By: Jane Doe <jane@example.com>",
			want: map[string][]string{
				"Reviewed-by":    {"Reviewer <reviewer@example.com>"},
				"Signed-off-by":  {"Maintainer <maintainer@example.com>"},
				"Co-Authored-By": {"Jane Doe <jane@example.com>"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ParseTrailers(tt.message)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseTrailers() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
