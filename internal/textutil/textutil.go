// Package textutil provides small rune-safe string helpers shared by the
// TUI packages. Byte-indexed slicing (s[:n]) splits multi-byte UTF-8
// sequences mid-rune; these helpers operate on runes instead.
//
// Note: limits are in runes, not terminal cells — East-Asian wide
// characters still occupy two cells. That matches the surrounding layout
// code, which pads with fmt %-*s (also not cell-aware).
package textutil

// Truncate shortens s to at most max runes. When truncation is needed the
// result is exactly max runes and ends with a single ellipsis rune ("…").
// A max below 1 returns the empty string.
func Truncate(s string, max int) string {
	if max < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// TruncateLeft shortens s to at most max runes, keeping the tail of the
// string and prefixing a single ellipsis rune ("…") when truncation is
// needed (e.g. for filesystem paths where the end matters most).
// A max below 1 returns the empty string.
func TruncateLeft(s string, max int) string {
	if max < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return "…" + string(r[len(r)-max+1:])
}
