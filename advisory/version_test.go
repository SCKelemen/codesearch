package advisory

import "testing"

func TestParseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  Version
	}{
		{input: "1.2.3", want: Version{Major: 1, Minor: 2, Patch: 3, Raw: "1.2.3"}},
		{input: "v1.2.3", want: Version{Major: 1, Minor: 2, Patch: 3, Raw: "v1.2.3"}},
		{input: "1.2", want: Version{Major: 1, Minor: 2, Patch: 0, Raw: "1.2"}},
		{input: "1", want: Version{Major: 1, Minor: 0, Patch: 0, Raw: "1"}},
		{input: "1.2.3-beta", want: Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "beta", Raw: "1.2.3-beta"}},
		{input: "01.02.03", want: Version{Major: 1, Minor: 2, Patch: 3, Raw: "01.02.03"}},
		{input: "0.0.0", want: Version{Major: 0, Minor: 0, Patch: 0, Raw: "0.0.0"}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			got, err := ParseVersion(test.input)
			if err != nil {
				t.Fatalf("ParseVersion(%q) error = %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("ParseVersion(%q) = %#v, want %#v", test.input, got, test.want)
			}
		})
	}
}

func TestParseVersionErrors(t *testing.T) {
	t.Parallel()

	inputs := []string{"", "abc", "1.2.3.4"}
	for _, input := range inputs {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseVersion(input); err == nil {
				t.Fatalf("ParseVersion(%q) error = nil, want non-nil", input)
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{name: "equal", left: "1.2.3", right: "1.2.3", want: 0},
		{name: "less", left: "1.2.2", right: "1.2.3", want: -1},
		{name: "greater", left: "1.3.0", right: "1.2.9", want: 1},
		{name: "prerelease less than release", left: "1.2.3-beta", right: "1.2.3", want: -1},
		{name: "prerelease lexical", left: "1.2.3-alpha", right: "1.2.3-beta", want: -1},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			left, err := ParseVersion(test.left)
			if err != nil {
				t.Fatalf("ParseVersion(%q) error = %v", test.left, err)
			}
			right, err := ParseVersion(test.right)
			if err != nil {
				t.Fatalf("ParseVersion(%q) error = %v", test.right, err)
			}
			if got := left.Compare(right); got != test.want {
				t.Fatalf("Compare(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
			}
		})
	}
}

func TestVersionOrderingHelpers(t *testing.T) {
	t.Parallel()

	left, err := ParseVersion("1.2.3")
	if err != nil {
		t.Fatalf("ParseVersion(left) error = %v", err)
	}
	right, err := ParseVersion("1.2.4")
	if err != nil {
		t.Fatalf("ParseVersion(right) error = %v", err)
	}

	if !left.LessThan(right) {
		t.Fatal("LessThan() = false, want true")
	}
	if !right.GreaterThanOrEqual(left) {
		t.Fatal("GreaterThanOrEqual() = false, want true")
	}
}

func TestVersionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{input: "1.2.3", want: "1.2.3"},
		{input: "01.02.03", want: "1.2.3"},
		{input: "1.2.3-beta", want: "1.2.3-beta"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			version, err := ParseVersion(test.input)
			if err != nil {
				t.Fatalf("ParseVersion(%q) error = %v", test.input, err)
			}
			if got := version.String(); got != test.want {
				t.Fatalf("String() = %q, want %q", got, test.want)
			}
		})
	}
}
