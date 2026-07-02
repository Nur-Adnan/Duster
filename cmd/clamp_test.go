package cmd

import (
	"testing"
	"unicode/utf8"
)

// clampHead/clampTail must never split a multi-byte rune (byte slicing left a
// � at the cut on non-ASCII paths) and must never panic on short input.
func TestClampRuneSafe(t *testing.T) {
	long := "Naïve café résumé project — файл — 日本語ディレクトリ path segment here"

	for _, w := range []int{4, 10, 20, 30, 42, 80} {
		h := clampHead(long, w)
		if !utf8.ValidString(h) {
			t.Errorf("clampHead(width=%d) produced invalid UTF-8: %q", w, h)
		}
		if utf8.RuneCountInString(h) > w {
			t.Errorf("clampHead(width=%d) exceeded width: %d runes", w, utf8.RuneCountInString(h))
		}
		tl := clampTail(long, w)
		if !utf8.ValidString(tl) {
			t.Errorf("clampTail(width=%d) produced invalid UTF-8: %q", w, tl)
		}
		if utf8.RuneCountInString(tl) > w {
			t.Errorf("clampTail(width=%d) exceeded width: %d runes", w, utf8.RuneCountInString(tl))
		}
	}

	// Short/empty input is returned unchanged, no panic.
	for _, s := range []string{"", "a", "short"} {
		if got := clampHead(s, 42); got != s {
			t.Errorf("clampHead(%q) = %q, want unchanged", s, got)
		}
		if got := clampTail(s, 42); got != s {
			t.Errorf("clampTail(%q) = %q, want unchanged", s, got)
		}
	}

	// ASCII path longer than width keeps the affix and the leaf/head.
	if got := clampTail("C:\\Users\\bob\\very\\deep\\node_modules\\pkg", 20); utf8.RuneCountInString(got) != 20 || got[:3] != "..." {
		t.Errorf("clampTail ASCII = %q (len %d)", got, utf8.RuneCountInString(got))
	}
	if got := clampHead("VeryLongApplicationName Enterprise Edition", 20); got[len(got)-3:] != "..." {
		t.Errorf("clampHead ASCII = %q", got)
	}
}
