package patch

import (
	"reflect"
	"strings"
	"testing"
)

func TestFormatUnifiedDiffSingleHunk(t *testing.T) {
	t.Parallel()

	fp := FilePatch{
		Path: "package.json",
		Hunks: []Hunk{{
			OldStart: 5,
			OldCount: 1,
			NewStart: 5,
			NewCount: 1,
			Lines: []Line{
				{Op: OpContext, Content: "  \"dependencies\": {"},
				{Op: OpDelete, Content: "    \"lodash\": \"4.17.20\""},
				{Op: OpAdd, Content: "    \"lodash\": \"4.17.21\""},
			},
		}},
	}

	got := FormatUnifiedDiff(fp)
	want := strings.Join([]string{
		"--- a/package.json",
		"+++ b/package.json",
		"@@ -5,1 +5,1 @@",
		"   \"dependencies\": {",
		"-    \"lodash\": \"4.17.20\"",
		"+    \"lodash\": \"4.17.21\"",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("FormatUnifiedDiff() mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatUnifiedDiffMultipleHunks(t *testing.T) {
	t.Parallel()

	fp := FilePatch{
		Path: "requirements.txt",
		Hunks: []Hunk{
			{
				OldStart: 1,
				OldCount: 1,
				NewStart: 1,
				NewCount: 1,
				Lines: []Line{
					{Op: OpDelete, Content: "requests==2.25.0"},
					{Op: OpAdd, Content: "requests==2.31.0"},
				},
			},
			{
				OldStart: 4,
				OldCount: 1,
				NewStart: 4,
				NewCount: 1,
				Lines: []Line{
					{Op: OpDelete, Content: "urllib3==1.26.0"},
					{Op: OpAdd, Content: "urllib3==1.26.18"},
				},
			},
		},
	}

	got := FormatUnifiedDiff(fp)
	if !strings.Contains(got, "@@ -1,1 +1,1 @@") {
		t.Fatalf("FormatUnifiedDiff() missing first hunk: %q", got)
	}
	if !strings.Contains(got, "@@ -4,1 +4,1 @@") {
		t.Fatalf("FormatUnifiedDiff() missing second hunk: %q", got)
	}
}

func TestFormatPatchMultiFile(t *testing.T) {
	t.Parallel()

	p := Patch{
		AdvisoryID:  "GHSA-1234",
		Description: "Update vulnerable dependencies",
		Files: []FilePatch{
			{Path: "package.json", Hunks: []Hunk{{OldStart: 1, OldCount: 1, NewStart: 1, NewCount: 1, Lines: []Line{{Op: OpDelete, Content: "old"}, {Op: OpAdd, Content: "new"}}}}},
			{Path: "requirements.txt", Hunks: []Hunk{{OldStart: 2, OldCount: 1, NewStart: 2, NewCount: 1, Lines: []Line{{Op: OpDelete, Content: "before"}, {Op: OpAdd, Content: "after"}}}}},
		},
	}

	got := FormatPatch(p)
	if !strings.Contains(got, "# Advisory: GHSA-1234") {
		t.Fatalf("FormatPatch() missing advisory header: %q", got)
	}
	if !strings.Contains(got, "--- a/package.json") || !strings.Contains(got, "--- a/requirements.txt") {
		t.Fatalf("FormatPatch() missing file diffs: %q", got)
	}
}

func TestParseUnifiedDiffRoundTrip(t *testing.T) {
	t.Parallel()

	original := FilePatch{
		Path: "go.mod",
		Hunks: []Hunk{{
			OldStart: 6,
			OldCount: 2,
			NewStart: 6,
			NewCount: 2,
			Lines: []Line{
				{Op: OpContext, Content: "require ("},
				{Op: OpDelete, Content: "\tgithub.com/example/lib v1.0.0"},
				{Op: OpAdd, Content: "\tgithub.com/example/lib v1.0.1"},
			},
		}},
	}

	parsed, err := ParseUnifiedDiff(FormatUnifiedDiff(original))
	if err != nil {
		t.Fatalf("ParseUnifiedDiff() error = %v", err)
	}
	if !reflect.DeepEqual(parsed, original) {
		t.Fatalf("ParseUnifiedDiff() mismatch\n got: %#v\nwant: %#v", parsed, original)
	}
}

func TestParseUnifiedDiffRealWorldSnippet(t *testing.T) {
	t.Parallel()

	diff := strings.Join([]string{
		"--- a/package.json",
		"+++ b/package.json",
		"@@ -10,2 +10,2 @@",
		"   \"dependencies\": {",
		"-    \"lodash\": \"4.17.20\",",
		"+    \"lodash\": \"4.17.21\",",
		"   }",
		"",
	}, "\n")

	parsed, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff() error = %v", err)
	}
	if parsed.Path != "package.json" {
		t.Fatalf("ParseUnifiedDiff() path = %q, want %q", parsed.Path, "package.json")
	}
	if len(parsed.Hunks) != 1 || len(parsed.Hunks[0].Lines) != 4 {
		t.Fatalf("ParseUnifiedDiff() unexpected hunk contents: %#v", parsed.Hunks)
	}
}

func TestFormatUnifiedDiffEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty patch", func(t *testing.T) {
		t.Parallel()

		got := FormatUnifiedDiff(FilePatch{Path: "empty.txt"})
		want := "--- a/empty.txt\n+++ b/empty.txt\n"
		if got != want {
			t.Fatalf("FormatUnifiedDiff() = %q, want %q", got, want)
		}
	})

	t.Run("created file", func(t *testing.T) {
		t.Parallel()

		fp := FilePatch{Path: "new.txt", Created: true, Hunks: []Hunk{{OldStart: 0, OldCount: 0, NewStart: 1, NewCount: 1, Lines: []Line{{Op: OpAdd, Content: "hello"}}}}}
		parsed, err := ParseUnifiedDiff(FormatUnifiedDiff(fp))
		if err != nil {
			t.Fatalf("ParseUnifiedDiff() error = %v", err)
		}
		if !parsed.Created || parsed.Deleted {
			t.Fatalf("ParseUnifiedDiff() created flags = %#v", parsed)
		}
	})

	t.Run("deleted file", func(t *testing.T) {
		t.Parallel()

		fp := FilePatch{Path: "old.txt", Deleted: true, Hunks: []Hunk{{OldStart: 1, OldCount: 1, NewStart: 0, NewCount: 0, Lines: []Line{{Op: OpDelete, Content: "goodbye"}}}}}
		parsed, err := ParseUnifiedDiff(FormatUnifiedDiff(fp))
		if err != nil {
			t.Fatalf("ParseUnifiedDiff() error = %v", err)
		}
		if !parsed.Deleted || parsed.Created {
			t.Fatalf("ParseUnifiedDiff() deleted flags = %#v", parsed)
		}
	})
}
