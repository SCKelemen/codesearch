package store

import (
	"testing"
)

func TestNewTrigram(t *testing.T) {
	t.Parallel()

	tri := NewTrigram('a', 'b', 'c')
	b := tri.Bytes()
	if b[0] != 'a' || b[1] != 'b' || b[2] != 'c' {
		t.Errorf("got bytes %v, want [a b c]", b)
	}
}

func TestTrigramString(t *testing.T) {
	t.Parallel()

	tri := NewTrigram('f', 'o', 'o')
	if tri.String() != "foo" {
		t.Errorf("got %q, want %q", tri.String(), "foo")
	}
}

func TestParseTrigram(t *testing.T) {
	t.Parallel()

	tri, err := ParseTrigram("abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tri.String() != "abc" {
		t.Errorf("got %q, want %q", tri.String(), "abc")
	}
}

func TestParseTrigramInvalid(t *testing.T) {
	t.Parallel()

	_, err := ParseTrigram("ab")
	if err == nil {
		t.Error("expected error for 2-byte input")
	}

	_, err = ParseTrigram("abcd")
	if err == nil {
		t.Error("expected error for 4-byte input")
	}
}

func TestTrigramRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []string{"abc", "xyz", "   ", "123", "!@#"}
	for _, s := range tests {
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			tri, err := ParseTrigram(s)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if tri.String() != s {
				t.Errorf("round trip failed: got %q, want %q", tri.String(), s)
			}
		})
	}
}
