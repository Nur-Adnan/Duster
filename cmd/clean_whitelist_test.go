package cmd

import "testing"

func TestWhitelistSetAcceptsBothVocabularies(t *testing.T) {
	set, unknown := whitelistSet([]string{"Chrome", " logs ", "npm", "bogus", ""})

	for _, id := range []string{"chrome", "browsers", "logs", "wer", "logfiles", "npm"} {
		if !set[id] {
			t.Errorf("whitelist should protect %q", id)
		}
	}
	if set["temp"] {
		t.Error("whitelist protected a category nobody named")
	}
	if len(unknown) != 1 || unknown[0] != "bogus" {
		t.Errorf("unknown = %q, want only \"bogus\"", unknown)
	}
}
