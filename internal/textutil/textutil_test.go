package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"empty", "", 5, ""},
		{"fits exactly", "abcde", 5, "abcde"},
		{"shorter", "abc", 5, "abc"},
		{"ascii truncated", "abcdef", 5, "abcd…"},
		{"multibyte fits by runes not bytes", "ééééé", 5, "ééééé"},
		{"multibyte truncated", "éééééé", 5, "éééé…"},
		{"cjk truncated", "気気気気気気", 4, "気気気…"},
		{"mixed truncated", "aéb気cd", 4, "aéb…"},
		{"max one", "abcdef", 1, "…"},
		{"max zero", "abcdef", 0, ""},
		{"max negative", "abcdef", -3, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.in, tt.max)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("Truncate(%q, %d) = %q is invalid UTF-8", tt.in, tt.max, got)
			}
			if n := utf8.RuneCountInString(got); tt.max >= 0 && n > tt.max {
				t.Errorf("Truncate(%q, %d) is %d runes, want <= %d", tt.in, tt.max, n, tt.max)
			}
		})
	}
}

func TestTruncateLeft(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"empty", "", 5, ""},
		{"fits exactly", "abcde", 5, "abcde"},
		{"shorter", "abc", 5, "abc"},
		{"ascii truncated keeps tail", "abcdef", 5, "…cdef"},
		{"multibyte truncated keeps tail", "éééééé", 5, "…éééé"},
		{"path tail preserved", "/home/jürgen/Hearthstone", 14, "…n/Hearthstone"},
		{"max one", "abcdef", 1, "…"},
		{"max zero", "abcdef", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateLeft(tt.in, tt.max)
			if got != tt.want {
				t.Errorf("TruncateLeft(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("TruncateLeft(%q, %d) = %q is invalid UTF-8", tt.in, tt.max, got)
			}
		})
	}
}

// TestTruncate_NeverSplitsRunes fuzz-lite sweep across every cut point of a
// fully multi-byte string.
func TestTruncate_NeverSplitsRunes(t *testing.T) {
	s := strings.Repeat("é気🎉", 10) // 2-, 3-, and 4-byte runes
	for max := 0; max <= utf8.RuneCountInString(s)+2; max++ {
		if got := Truncate(s, max); !utf8.ValidString(got) {
			t.Fatalf("Truncate(s, %d) produced invalid UTF-8: %q", max, got)
		}
		if got := TruncateLeft(s, max); !utf8.ValidString(got) {
			t.Fatalf("TruncateLeft(s, %d) produced invalid UTF-8: %q", max, got)
		}
	}
}
