package cmd

import "strings"

// whitelistAliases maps the extra names --whitelist accepts onto IDs in both
// vocabularies: the CLI's getCategories IDs and the clean TUI's items, whose
// "logs" spans wer + logfiles. Protection errs toward skipping more.
var whitelistAliases = map[string][]string{
	"chrome":   {"browsers"},
	"edge":     {"browsers"},
	"brave":    {"browsers"},
	"firefox":  {"browsers"},
	"wer":      {"logs"},
	"logfiles": {"logs"},
	"logs":     {"wer", "logfiles"},
}

// whitelistSet expands --whitelist values into the set of protected IDs, and
// returns the values that match no category or alias so callers can warn
// instead of silently protecting nothing.
func whitelistSet(ids []string) (map[string]bool, []string) {
	known := map[string]bool{}
	for _, c := range getCategories() {
		known[c.ID] = true
	}
	set := map[string]bool{}
	var unknown []string
	for _, raw := range ids {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		aliases, isAlias := whitelistAliases[id]
		if !known[id] && !isAlias {
			unknown = append(unknown, raw)
			continue
		}
		set[id] = true
		for _, a := range aliases {
			set[a] = true
		}
	}
	return set, unknown
}
