package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// du restore lists, restores and empties what the undo window is holding:
// items other commands moved into the quarantine (cmd/quarantine.go) instead
// of deleting outright, kept for quarantineKeep before sweepQuarantine expires
// them. du restore itself applies expiry only (sweepExpiredOnly).

var (
	restoreJSON   bool
	restoreDryRun bool
	restoreEmptyF bool
	restoreYes    bool
	restoreItem   int
)

var RestoreCmd = &cobra.Command{
	Use:   "restore [n|id]",
	Short: "List, restore or empty what Duster kept instead of deleting",
	Long: `User-facing deletes (purge, uninstall leftovers, old installers) are moved
into a per-volume quarantine instead of being removed outright, and kept there
for 7 days. Run without arguments to list what is kept, du restore <n> to put
a session back, or du restore --empty to delete it for good. <n> is the number
du restore lists, or the session id du restore --json prints (an id always
names the same session, even after the numbering shifts).`,
	Args: cobra.MaximumNArgs(1),
	Run:  executeRestore,
}

func init() {
	RestoreCmd.Flags().BoolVar(&restoreJSON, "json", false, "Print machine-readable JSON")
	RestoreCmd.Flags().BoolVar(&restoreDryRun, "dry-run", false, "Show what would happen; change nothing")
	RestoreCmd.Flags().BoolVar(&restoreEmptyF, "empty", false, "Delete kept session(s) for good instead of restoring them")
	RestoreCmd.Flags().BoolVar(&restoreYes, "yes", false, "Don't prompt before emptying")
	RestoreCmd.Flags().IntVar(&restoreItem, "item", 0, "Restore only this item number within the session")
}

func restoreFail(err error) {
	if restoreJSON {
		b, _ := json.MarshalIndent(map[string]string{"error": err.Error()}, "", "  ")
		fmt.Println(string(b))
	} else {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	os.Exit(1)
}

func executeRestore(c *cobra.Command, args []string) {
	// A dry run changes nothing, so it does not apply retention either. Only
	// expiry applies here, never the low-space pass: the user came to restore,
	// so a fresh session is never removed before they can get it back.
	if !restoreDryRun {
		rep := sweepQuarantine(time.Now(), sweepExpiredOnly)
		if l := rep.expiredLine(); l != "" {
			// Kept off stdout under --json, so the JSON stays parseable.
			if restoreJSON {
				fmt.Fprintln(os.Stderr, l)
			} else {
				fmt.Println(l)
			}
		}
		for _, e := range rep.Errs {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", e)
		}
	}

	itemGiven := c.Flags().Changed("item")
	if itemGiven && len(args) == 0 {
		restoreFail(errors.New("--item needs a session number: du restore <n> --item <k>"))
	}
	if itemGiven && restoreEmptyF {
		restoreFail(errors.New("--item cannot be used with --empty"))
	}

	rs := groupSessions(loadKeptSessions(quarantineRoots()))

	if restoreEmptyF {
		executeRestoreEmpty(rs, args)
		return
	}

	if len(args) == 0 {
		if restoreJSON {
			printRestoreListJSON(rs)
		} else {
			renderRestoreList(os.Stdout, rs, time.Now())
		}
		return
	}

	session, n, err := pickRestoreSession(rs, args[0])
	if err != nil {
		restoreFail(err)
	}
	if err := restoreRequestError(session, n, restoreItem, itemGiven); err != nil {
		restoreFail(err)
	}
	executeRestoreSession(session, n, itemGiven)
}

// damaged reports whether any of the session's folders has no readable
// manifest, so its items cannot be listed or restored.
func (r restoreSession) damaged() bool {
	for _, k := range r.Parts {
		if k.Damaged {
			return true
		}
	}
	return false
}

func damagedSessionMsg(n int) string {
	return fmt.Sprintf("session %d is damaged: its items cannot be listed or restored; empty it with du restore --empty %d", n, n)
}

// restoreRequestError says why du restore <n> [--item k] cannot run: a session
// with nothing but damaged folders, or an item number outside 1..len(Items()).
func restoreRequestError(r restoreSession, n, item int, itemGiven bool) error {
	items := len(r.Items())
	if items == 0 && r.damaged() {
		return errors.New(damagedSessionMsg(n))
	}
	if itemGiven && (item < 1 || item > items) {
		return fmt.Errorf("session %d has no item %d: it holds %s, numbered from 1 (du restore --json lists them)", n, item, plural(items, "item"))
	}
	return nil
}

// pickRestoreSession resolves the 1-based number `du restore` printed, or a
// session id from `du restore --json`, to the session it refers to and its
// current number. An id never matches a number (ids hold a "-"), and it
// names the same session however the list has shifted since it was printed.
func pickRestoreSession(rs []restoreSession, arg string) (restoreSession, int, error) {
	if n, err := strconv.Atoi(arg); err == nil {
		if n < 1 || n > len(rs) {
			return restoreSession{}, 0, fmt.Errorf("no kept session %s: run du restore to list them", arg)
		}
		return rs[n-1], n, nil
	}
	for i, r := range rs {
		if arg != "" && r.ID == arg {
			return r, i + 1, nil
		}
	}
	return restoreSession{}, 0, fmt.Errorf("no kept session %s: run du restore to list them", arg)
}

const restoreDateLayout = "Jan 2, 15:04"
const restoreExpiryLayout = "Jan 2"

// renderRestoreList is `du restore`'s text listing of what is kept.
func renderRestoreList(w io.Writer, rs []restoreSession, now time.Time) {
	if len(rs) == 0 {
		fmt.Fprintln(w, "Nothing is kept. User-facing deletes (purge, uninstall leftovers, old installers) are kept here for 7 days.")
		return
	}
	fmt.Fprintln(w, "Kept by Duster (restorable for 7 days):")
	var held int64
	for i, r := range rs {
		items := len(r.Items())
		count, note := plural(items, "item"), ""
		switch {
		case items == 0 && r.damaged():
			count = "damaged"
		case r.damaged():
			note = "  (part damaged)"
		}
		expires := r.Created.Add(quarantineKeep).Local().Format(restoreExpiryLayout)
		fmt.Fprintf(w, "  %d  %s  %-10s %-10s %s  expires %s%s\n",
			i+1, r.Created.Local().Format(restoreDateLayout), r.Command,
			count, strings.TrimSpace(formatBytes(r.Size())), expires, note)
		held += r.Size()
	}
	fmt.Fprintf(w, "Held: %s. Restore with: du restore <n>   Empty now: du restore --empty\n", strings.TrimSpace(formatBytes(held)))
}

type restoreItemJSON struct {
	Number int    `json:"number"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Dir    bool   `json:"dir"`
}

type restoreSessionJSON struct {
	Number  int               `json:"number"`
	ID      string            `json:"id"`
	Command string            `json:"command"`
	Created time.Time         `json:"created"`
	Expires time.Time         `json:"expires"`
	Items   []restoreItemJSON `json:"items"`
	Size    int64             `json:"size"`
	Damaged bool              `json:"damaged,omitempty"`
}

func printRestoreListJSON(rs []restoreSession) {
	sessions := make([]restoreSessionJSON, 0, len(rs))
	for i, r := range rs {
		items := make([]restoreItemJSON, 0, len(r.Items()))
		for n, ref := range r.Items() {
			items = append(items, restoreItemJSON{Number: n + 1, Path: ref.Item.Path, Size: ref.Item.Size, Dir: ref.Item.Dir})
		}
		sessions = append(sessions, restoreSessionJSON{
			Number: i + 1, ID: r.ID, Command: r.Command, Created: r.Created,
			Expires: r.Created.Add(quarantineKeep), Items: items, Size: r.Size(), Damaged: r.damaged(),
		})
	}
	b, _ := json.MarshalIndent(map[string][]restoreSessionJSON{"sessions": sessions}, "", "  ")
	fmt.Println(string(b))
}

// executeRestoreSession runs du restore <n> [--item k] [--dry-run] [--json].
func executeRestoreSession(session restoreSession, n int, itemGiven bool) {
	item := 0
	if itemGiven {
		item = restoreItem
	}
	results := restoreItems(session, item, restoreDryRun)
	if !itemGiven {
		results = append(results, damagedPartResults(session, n)...)
	}
	failed := false
	for _, r := range results {
		if r.Status == "failed" {
			failed = true
		}
	}
	if restoreJSON {
		b, _ := json.MarshalIndent(map[string]any{"results": results, "failed": failed}, "", "  ")
		fmt.Println(string(b))
	} else {
		printRestoreResults(os.Stdout, results)
	}
	if failed {
		os.Exit(1)
	}
}

// damagedPartResults reports each damaged folder of a partly readable session
// as failed, so restoring "everything" never silently leaves part of it behind.
func damagedPartResults(r restoreSession, n int) []restoreResult {
	var out []restoreResult
	for _, k := range r.Parts {
		if k.Damaged {
			out = append(out, restoreResult{Path: k.Dir, Status: "failed", Reason: damagedSessionMsg(n)})
		}
	}
	return out
}

func printRestoreResults(w io.Writer, results []restoreResult) {
	var restored, would, skipped, failed int
	var restoredSize int64
	for _, r := range results {
		switch r.Status {
		case "restored":
			restored++
			restoredSize += r.Size
			fmt.Fprintf(w, "restored: %s\n", r.Path)
		case "would restore":
			would++
			restoredSize += r.Size
			fmt.Fprintf(w, "would restore: %s\n", r.Path)
		case "skipped":
			skipped++
			fmt.Fprintf(w, "skipped: %s (%s)\n", r.Path, r.Reason)
		default: // failed
			failed++
			fmt.Fprintf(w, "failed: %s (%s)\n", r.Path, r.Reason)
		}
	}
	verb := "Restored"
	if would > 0 {
		verb = "Would restore"
	}
	n := restored + would
	summary := fmt.Sprintf("%s %s (%s)", verb, plural(n, "item"), strings.TrimSpace(formatBytes(restoredSize)))
	if skipped > 0 {
		summary += fmt.Sprintf(", %d skipped", skipped)
	}
	if failed > 0 {
		summary += fmt.Sprintf(", %d failed", failed)
	}
	fmt.Fprintln(w, summary+".")
}

// restoreSessionLine names one session for a prompt: command, time, item
// count and size.
func restoreSessionLine(r restoreSession) string {
	count := plural(len(r.Items()), "item")
	if len(r.Items()) == 0 && r.damaged() {
		count = "damaged"
	}
	return fmt.Sprintf("%s from %s, %s, %s", r.Command, r.Created.Local().Format(restoreDateLayout),
		count, strings.TrimSpace(formatBytes(r.Size())))
}

// emptyPrompt is du restore --empty's question: it names each session that
// would be deleted for good, so the user sees exactly what they confirm.
func emptyPrompt(targets []restoreSession) string {
	var b strings.Builder
	if len(targets) == 1 {
		fmt.Fprintf(&b, "Delete %s for good? [y/N] ", restoreSessionLine(targets[0]))
		return b.String()
	}
	var size int64
	b.WriteString("This deletes for good:\n")
	for _, r := range targets {
		fmt.Fprintf(&b, "  %s\n", restoreSessionLine(r))
		size += r.Size()
	}
	fmt.Fprintf(&b, "Delete these %s (%s) kept by Duster for good? [y/N] ", plural(len(targets), "session"), strings.TrimSpace(formatBytes(size)))
	return b.String()
}

// executeRestoreEmpty runs du restore --empty [n|id].
func executeRestoreEmpty(rs []restoreSession, args []string) {
	var targets []restoreSession
	if len(args) > 0 {
		session, _, err := pickRestoreSession(rs, args[0])
		if err != nil {
			restoreFail(err)
		}
		targets = []restoreSession{session}
	} else {
		targets = rs
	}

	var size int64
	sessionCount := len(targets)
	for _, r := range targets {
		size += r.Size()
	}

	if sessionCount == 0 {
		if restoreJSON {
			b, _ := json.MarshalIndent(map[string]any{"emptied": 0, "size": 0}, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Println("Nothing to empty.")
		}
		return
	}

	if restoreDryRun {
		msg := fmt.Sprintf("Would empty %s (%s).", plural(sessionCount, "session"), strings.TrimSpace(formatBytes(size)))
		if restoreJSON {
			b, _ := json.MarshalIndent(map[string]any{"would_empty": sessionCount, "size": size}, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Println(msg)
		}
		return
	}

	if !restoreYes {
		if restoreJSON || isPiped() {
			restoreFail(errors.New("use --yes to empty the quarantine without a prompt"))
		}
		fmt.Print(emptyPrompt(targets))
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		if line != "y" && line != "yes" {
			fmt.Println("Cancelled.")
			return
		}
	}

	if err := emptySessions(targets); err != nil {
		restoreFail(err)
	}
	if restoreJSON {
		b, _ := json.MarshalIndent(map[string]any{"emptied": sessionCount, "size": size}, "", "  ")
		fmt.Println(string(b))
	} else {
		fmt.Printf("Emptied %s (%s).\n", plural(sessionCount, "session"), strings.TrimSpace(formatBytes(size)))
	}
}
