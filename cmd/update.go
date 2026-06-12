package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/internal/logging"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// Flags
var (
	upJSON  bool
	upCheck bool
	upForce bool
)

// Release endpoint and verification constants.
const (
	updateRepoOwner   = "Nur-Adnan"
	updateRepoName    = "Duster"
	updateAPIURL      = "https://api.github.com/repos/" + updateRepoOwner + "/" + updateRepoName + "/releases/latest"
	checksumsFileName = "checksums-sha256.txt"

	maxReleaseJSONBytes = 4 << 20   // 4 MiB cap on the API response
	maxChecksumBytes    = 1 << 20   // 1 MiB cap on the checksums file
	maxArchiveBytes     = 256 << 20 // 256 MiB cap on the release archive
	maxBinaryBytes      = 256 << 20 // 256 MiB cap on the extracted binary

	updateHTTPTimeout = 30 * time.Second
	downloadTimeout   = 5 * time.Minute
)

// Premium Lipgloss Styles (prefixed to avoid package conflicts)
var (
	upTealColor  = lipgloss.Color("#008080")
	upCyanColor  = lipgloss.Color("#00FFFF")
	upGrayColor  = lipgloss.Color("#666666")
	upRedColor   = lipgloss.Color("#FF0000")
	upGreenColor = lipgloss.Color("#00FF00")

	upHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(upCyanColor).
			Padding(0, 1)

	upBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(upTealColor).
			Padding(1, 2).
			Width(80)

	upFooterStyle = lipgloss.NewStyle().
			Foreground(upGrayColor).
			PaddingTop(1).
			PaddingLeft(2)

	upDividerStyle = lipgloss.NewStyle().
			Foreground(upGrayColor)

	upSuccessStyle = lipgloss.NewStyle().Bold(true).Foreground(upGreenColor)
	upFailStyle    = lipgloss.NewStyle().Bold(true).Foreground(upRedColor)
)

type updateState int

const (
	stateUpIdle updateState = iota
	stateUpChecking
	stateUpDownloading
	stateUpSwapping
	stateUpFinished
)

type releaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

type releaseMetadata struct {
	TagName     string         `json:"tag_name"`
	PublishedAt string         `json:"published_at"`
	Body        string         `json:"body"`
	Size        int64          `json:"binary_size_bytes"`
	Assets      []releaseAsset `json:"assets,omitempty"`
}

var UpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check and update Duster to the latest version",
	Long: `Queries the GitHub releases API to compare the local version with the latest release,
verifies the download against its published SHA-256 checksum, and performs a
Windows-native binary swap to safely self-update the utility in-place.`,
	Run: executeUpdate,
}

func init() {
	UpdateCmd.Flags().BoolVar(&upJSON, "json", false, "Output update availability details as JSON and exit immediately")
	UpdateCmd.Flags().BoolVarP(&upCheck, "check", "c", false, "Verify if an update is available without downloading it")
	UpdateCmd.Flags().BoolVarP(&upForce, "force", "f", false, "Force executable update even if already on the latest version")
}

func executeUpdate(cmd *cobra.Command, args []string) {
	// Headless / JSON snapshot execution
	if upJSON || isPiped() {
		runHeadlessUpdate()
		return
	}

	m := initialUpdateModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running updater TUI: %v\n", err)
		os.Exit(1)
	}
}

type updateModel struct {
	state          updateState
	currentVersion string
	latestVersion  string
	latestRelease  releaseMetadata
	updateFound    bool
	statusMsg      string
	width          int
	height         int
}

type checkCompleteMsg struct {
	release releaseMetadata
	err     error
}

type downloadCompleteMsg struct {
	bytes []byte
	err   error
}

type swapCompleteMsg struct {
	err error
}

func initialUpdateModel() updateModel {
	return updateModel{
		state:          stateUpIdle,
		currentVersion: AppVersion,
		latestVersion:  AppVersion,
		updateFound:    false,
	}
}

func (m updateModel) Init() tea.Cmd {
	return nil
}

func newUpdateHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// Never follow a redirect off HTTPS.
			if req.URL.Scheme != "https" {
				return fmt.Errorf("refusing insecure redirect to %s", req.URL)
			}
			return nil
		},
	}
}

// fetchLatestRelease queries the GitHub releases API for the newest published release.
func fetchLatestRelease() (releaseMetadata, error) {
	client := newUpdateHTTPClient(updateHTTPTimeout)

	req, err := http.NewRequest(http.MethodGet, updateAPIURL, nil)
	if err != nil {
		return releaseMetadata{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "duster-updater/"+AppVersion)

	resp, err := client.Do(req)
	if err != nil {
		return releaseMetadata{}, fmt.Errorf("cannot reach GitHub releases API: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// proceed
	case http.StatusForbidden, http.StatusTooManyRequests:
		return releaseMetadata{}, fmt.Errorf("GitHub API rate limit reached — please try again later")
	case http.StatusNotFound:
		return releaseMetadata{}, fmt.Errorf("no published releases found for %s/%s", updateRepoOwner, updateRepoName)
	default:
		return releaseMetadata{}, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var rel releaseMetadata
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseJSONBytes)).Decode(&rel); err != nil {
		return releaseMetadata{}, fmt.Errorf("invalid release metadata: %w", err)
	}
	if rel.TagName == "" {
		return releaseMetadata{}, fmt.Errorf("release metadata is missing a tag name")
	}

	// Surface the size of the archive matching this platform for display purposes.
	if asset, err := selectArchiveAsset(rel); err == nil {
		rel.Size = asset.Size
	}
	return rel, nil
}

// selectArchiveAsset picks the release zip matching this OS/architecture.
func selectArchiveAsset(rel releaseMetadata) (releaseAsset, error) {
	version := strings.TrimPrefix(rel.TagName, "v")
	wanted := fmt.Sprintf("duster-%s-windows-%s.zip", version, runtime.GOARCH)
	for _, a := range rel.Assets {
		if strings.EqualFold(a.Name, wanted) {
			return a, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("release %s has no asset named %q", rel.TagName, wanted)
}

func selectChecksumsAsset(rel releaseMetadata) (releaseAsset, error) {
	for _, a := range rel.Assets {
		if strings.EqualFold(a.Name, checksumsFileName) {
			return a, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("release %s does not publish %s", rel.TagName, checksumsFileName)
}

// downloadAsset fetches a release asset over HTTPS with a hard size limit.
func downloadAsset(asset releaseAsset, maxBytes int64) ([]byte, error) {
	if !strings.HasPrefix(strings.ToLower(asset.DownloadURL), "https://") {
		return nil, fmt.Errorf("refusing non-HTTPS download URL for %s", asset.Name)
	}

	client := newUpdateHTTPClient(downloadTimeout)
	req, err := http.NewRequest(http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "duster-updater/"+AppVersion)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download of %s failed: %w", asset.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download of %s failed: HTTP %d", asset.Name, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("download of %s interrupted: %w", asset.Name, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("download of %s exceeds the %d byte safety limit", asset.Name, maxBytes)
	}
	return data, nil
}

// expectedChecksumFor parses a goreleaser checksums file ("<hex>  <name>" lines)
// and returns the SHA-256 hex digest recorded for the named asset.
func expectedChecksumFor(checksums []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		if strings.EqualFold(fields[1], assetName) {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("no checksum entry found for %s", assetName)
}

// extractBinaryFromZip pulls the du executable out of the release archive.
func extractBinaryFromZip(archive []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("release archive is not a valid zip: %w", err)
	}

	for _, f := range reader.File {
		name := strings.ToLower(f.Name)
		if name != "du.exe" && !strings.HasSuffix(name, "/du.exe") {
			continue
		}
		if f.UncompressedSize64 > maxBinaryBytes {
			return nil, fmt.Errorf("binary inside archive exceeds the %d byte safety limit", int64(maxBinaryBytes))
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()

		data, err := io.ReadAll(io.LimitReader(rc, maxBinaryBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxBinaryBytes {
			return nil, fmt.Errorf("binary inside archive exceeds the %d byte safety limit", int64(maxBinaryBytes))
		}
		// Sanity check: Windows executables start with the MZ header.
		if len(data) < 2 || data[0] != 'M' || data[1] != 'Z' {
			return nil, fmt.Errorf("extracted file is not a valid Windows executable")
		}
		return data, nil
	}
	return nil, fmt.Errorf("release archive does not contain du.exe")
}

// downloadVerifiedBinary downloads the platform archive, verifies it against the
// release's published SHA-256 checksum, and returns the extracted binary bytes.
func downloadVerifiedBinary(rel releaseMetadata) ([]byte, error) {
	archiveAsset, err := selectArchiveAsset(rel)
	if err != nil {
		return nil, err
	}
	checksumsAsset, err := selectChecksumsAsset(rel)
	if err != nil {
		// SECURITY: verification is mandatory. A release without checksums is not installable.
		return nil, err
	}

	checksums, err := downloadAsset(checksumsAsset, maxChecksumBytes)
	if err != nil {
		return nil, err
	}
	expected, err := expectedChecksumFor(checksums, archiveAsset.Name)
	if err != nil {
		return nil, err
	}

	archive, err := downloadAsset(archiveAsset, maxArchiveBytes)
	if err != nil {
		return nil, err
	}

	actual := sha256Hex(archive)
	if actual != expected {
		return nil, fmt.Errorf("SHA-256 mismatch for %s: expected %s, got %s — refusing to install",
			archiveAsset.Name, expected, actual)
	}

	return extractBinaryFromZip(archive)
}

// swapBinary atomically replaces the running executable with the verified bytes.
// The new binary is staged next to the current one (same volume) so both renames
// are atomic, and the original is restored if any step fails.
func swapBinary(newBytes []byte) error {
	currentExe, err := os.Executable()
	if err != nil {
		return err
	}

	stagedExe := currentExe + ".new"
	oldExe := currentExe + ".old"

	if err := os.WriteFile(stagedExe, newBytes, 0o755); err != nil {
		return fmt.Errorf("cannot stage new binary: %w", err)
	}

	_ = os.Remove(oldExe) // clear leftovers from a previous update
	if err := os.Rename(currentExe, oldExe); err != nil {
		_ = os.Remove(stagedExe)
		return fmt.Errorf("cannot move current binary aside: %w", err)
	}

	if err := os.Rename(stagedExe, currentExe); err != nil {
		// Roll back: put the original binary back in place.
		if rbErr := os.Rename(oldExe, currentExe); rbErr != nil {
			return fmt.Errorf("swap failed (%v) and rollback failed (%v) — restore %s manually", err, rbErr, oldExe)
		}
		_ = os.Remove(stagedExe)
		return fmt.Errorf("cannot activate new binary: %w", err)
	}

	// The old binary stays locked while this process runs; delete it after exit.
	scheduleDelayedDelete(oldExe)
	logUpOperation("self-update", currentExe, int64(len(newBytes)), true)
	return nil
}

// isNewerVersion reports whether latest is strictly newer than current,
// comparing dot-separated numeric components (pre-release suffixes ignored).
func isNewerVersion(latest, current string) bool {
	parse := func(v string) []int {
		v = strings.TrimPrefix(strings.TrimSpace(v), "v")
		if i := strings.IndexAny(v, "-+"); i >= 0 {
			v = v[:i]
		}
		var nums []int
		for _, part := range strings.Split(v, ".") {
			n, err := strconv.Atoi(part)
			if err != nil {
				return nil
			}
			nums = append(nums, n)
		}
		return nums
	}

	l, c := parse(latest), parse(current)
	if l == nil || c == nil {
		// Unparseable versions (e.g. dev builds): treat any difference as an update.
		return strings.TrimPrefix(latest, "v") != strings.TrimPrefix(current, "v")
	}
	for i := 0; i < len(l) || i < len(c); i++ {
		var lv, cv int
		if i < len(l) {
			lv = l[i]
		}
		if i < len(c) {
			cv = c[i]
		}
		if lv != cv {
			return lv > cv
		}
	}
	return false
}

func runCheckReleaseCmd() tea.Cmd {
	return func() tea.Msg {
		rel, err := fetchLatestRelease()
		return checkCompleteMsg{release: rel, err: err}
	}
}

func runDownloadBinaryCmd(rel releaseMetadata) tea.Cmd {
	return func() tea.Msg {
		data, err := downloadVerifiedBinary(rel)
		return downloadCompleteMsg{bytes: data, err: err}
	}
}

func runSwapBinaryCmd(newBytes []byte) tea.Cmd {
	return func() tea.Msg {
		return swapCompleteMsg{err: swapBinary(newBytes)}
	}
}

func (m updateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "enter", "y", "Y":
			if m.state == stateUpIdle {
				m.state = stateUpChecking
				m.statusMsg = "Retrieving latest release manifest..."
				return m, runCheckReleaseCmd()
			} else if m.state == stateUpChecking && m.updateFound {
				if upCheck {
					return m, tea.Quit
				}
				m.state = stateUpDownloading
				m.statusMsg = "Downloading and verifying release archive..."
				return m, runDownloadBinaryCmd(m.latestRelease)
			} else if m.state == stateUpFinished {
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case checkCompleteMsg:
		if msg.err != nil {
			m.state = stateUpFinished
			m.statusMsg = fmt.Sprintf("Error checking updates: %v", msg.err)
			return m, nil
		}

		m.latestRelease = msg.release
		m.latestVersion = strings.TrimPrefix(msg.release.TagName, "v")
		m.updateFound = isNewerVersion(m.latestVersion, m.currentVersion) || upForce

		m.state = stateUpChecking // transition display state within checking card
		if m.updateFound {
			m.statusMsg = fmt.Sprintf("New version available: %s", msg.release.TagName)
		} else {
			m.statusMsg = "Duster is already on the latest version!"
		}
		return m, nil

	case downloadCompleteMsg:
		if msg.err != nil {
			m.state = stateUpFinished
			m.statusMsg = fmt.Sprintf("Download failed: %v", msg.err)
			return m, nil
		}

		m.state = stateUpSwapping
		m.statusMsg = "Applying binary updates..."
		return m, runSwapBinaryCmd(msg.bytes)

	case swapCompleteMsg:
		m.state = stateUpFinished
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Update failed during installation: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Self-Update successful! Duster has been updated to v%s.", m.latestVersion)
		}
		return m, nil
	}

	return m, nil
}

func (m updateModel) View() string {
	var doc strings.Builder

	// Top Title banner
	doc.WriteString("\n")
	doc.WriteString(upHeaderStyle.Render("Duster Exe Self-Updater"))
	doc.WriteString("\n")
	doc.WriteString(upDividerStyle.Render("  ═══════════════════════════════════════════════════════════════════════\n\n"))

	var boxLayout strings.Builder

	switch m.state {
	case stateUpIdle:
		boxLayout.WriteString("  Update workflow preparing to query:\n\n")
		boxLayout.WriteString(fmt.Sprintf("    Current installed version : %s\n", upCyanText("v"+m.currentVersion)))
		boxLayout.WriteString(fmt.Sprintf("    Release source            : github.com/%s/%s\n\n", updateRepoOwner, updateRepoName))
		boxLayout.WriteString("  Press [Enter] to check for available updates, or [q] to Exit.")

	case stateUpChecking:
		boxLayout.WriteString("🔍  " + upCyanText("CHECKING FOR VERSION ANNOUNCEMENTS") + "\n\n")
		boxLayout.WriteString(fmt.Sprintf("  Status: %s\n\n", m.statusMsg))

		if m.updateFound {
			boxLayout.WriteString(upSuccessStyle.Render("  Update Details:\n"))
			boxLayout.WriteString(fmt.Sprintf("    Latest Release tag : %s\n", upWhiteText(m.latestRelease.TagName)))
			if len(m.latestRelease.PublishedAt) >= 10 {
				boxLayout.WriteString(fmt.Sprintf("    Release Date       : %s\n", upWhiteText(m.latestRelease.PublishedAt[:10])))
			}
			if m.latestRelease.Size > 0 {
				boxLayout.WriteString(fmt.Sprintf("    Download Size      : %s\n", upWhiteText(formatBytes(m.latestRelease.Size))))
			}
			boxLayout.WriteString("\n  Changelog summary:\n")
			changelogLines := strings.Split(m.latestRelease.Body, "\n")
			const maxChangelogLines = 12
			if len(changelogLines) > maxChangelogLines {
				changelogLines = changelogLines[:maxChangelogLines]
			}
			for _, line := range changelogLines {
				boxLayout.WriteString(fmt.Sprintf("    %s\n", upGrayText(line)))
			}
			boxLayout.WriteString("\n")
			if upCheck {
				boxLayout.WriteString("  [Check Only] Press [Enter] or [q] to exit.")
			} else {
				boxLayout.WriteString("  Do you wish to install the update? [y to Swap / n to Cancel]")
			}
		} else {
			boxLayout.WriteString("  ✓ Duster is fully optimized and up-to-date!\n\n")
			boxLayout.WriteString("  Press [q] or [esc] to exit.")
		}

	case stateUpDownloading:
		boxLayout.WriteString("⌛  " + upCyanText("DOWNLOADING VERIFIED RELEASE ARCHIVE") + "\n\n")
		boxLayout.WriteString(fmt.Sprintf("  Status: %s\n\n", m.statusMsg))
		boxLayout.WriteString("  Downloading over HTTPS and verifying SHA-256 checksum... Do NOT close this window.")

	case stateUpSwapping:
		boxLayout.WriteString("⚡  " + upSuccessStyle.Render("EXECUTING WINDOWS-NATIVE BINARY SWAP") + "\n\n")
		boxLayout.WriteString(fmt.Sprintf("  Status: %s\n\n", m.statusMsg))
		boxLayout.WriteString("  Replacing running executable. Spawning detached background tasks...")

	case stateUpFinished:
		if strings.Contains(m.statusMsg, "failed") || strings.Contains(m.statusMsg, "Error") {
			boxLayout.WriteString("✗  " + upFailStyle.Render("SELF-UPDATE WORKFLOW FINISHED") + "\n\n")
		} else {
			boxLayout.WriteString("✓  " + upSuccessStyle.Render("SELF-UPDATE WORKFLOW FINISHED") + "\n\n")
		}
		boxLayout.WriteString(fmt.Sprintf("  Result: %s\n\n", m.statusMsg))
		boxLayout.WriteString("  Press [q] or [esc] to return to the CLI shell.")
	}

	doc.WriteString(upBoxStyle.Render(boxLayout.String()))
	doc.WriteString("\n")

	// Footer instructions
	switch m.state {
	case stateUpIdle:
		doc.WriteString(upFooterStyle.Render("[Enter] Check Version  |  [q/esc] Exit Updater"))
	case stateUpChecking:
		if m.updateFound {
			if upCheck {
				doc.WriteString(upFooterStyle.Render("[Enter/q] Exit"))
			} else {
				doc.WriteString(upFooterStyle.Render("[y] Confirm Download  |  [n/esc] Cancel"))
			}
		} else {
			doc.WriteString(upFooterStyle.Render("[q/esc] Exit to Shell"))
		}
	case stateUpDownloading, stateUpSwapping:
		doc.WriteString(upFooterStyle.Render("Installing update... Do NOT interrupt."))
	case stateUpFinished:
		doc.WriteString(upFooterStyle.Render("[q/esc] Exit to Shell"))
	}

	return doc.String()
}

// logUpOperation delegates to the shared structured logging system.
func logUpOperation(action, target string, size int64, success bool) {
	logging.LogDestructiveOperation("update", action, target, size, success)
}

func runHeadlessUpdate() {
	currentVersion := AppVersion

	rel, checkErr := fetchLatestRelease()
	latestVersion := strings.TrimPrefix(rel.TagName, "v")
	updateAvailable := checkErr == nil && (isNewerVersion(latestVersion, currentVersion) || upForce)

	var swapErr error
	if updateAvailable && !upCheck {
		if data, err := downloadVerifiedBinary(rel); err != nil {
			swapErr = err
		} else {
			swapErr = swapBinary(data)
		}
	}

	statusStr := "UP_TO_DATE"
	switch {
	case checkErr != nil:
		statusStr = fmt.Sprintf("CHECK_FAILED: %v", checkErr)
	case updateAvailable && upCheck:
		statusStr = "NEW_VERSION_AVAILABLE"
	case updateAvailable && swapErr != nil:
		statusStr = fmt.Sprintf("INSTALLATION_FAILED: %v", swapErr)
	case updateAvailable:
		statusStr = "UPDATE_SUCCESSFULLY_INSTALLED"
	}

	payload := struct {
		CurrentVersion  string          `json:"current_version"`
		LatestVersion   string          `json:"latest_version"`
		UpdateAvailable bool            `json:"update_available"`
		Status          string          `json:"status"`
		ReleaseDetails  releaseMetadata `json:"release_details"`
		Timestamp       string          `json:"timestamp"`
	}{
		CurrentVersion:  currentVersion,
		LatestVersion:   latestVersion,
		UpdateAvailable: updateAvailable,
		Status:          statusStr,
		ReleaseDetails:  rel,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding update status: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

// Local helper style functions — delegate to canonical shared helpers
func upCyanText(s string) string  { return cyanText(s) }
func upWhiteText(s string) string { return whiteText(s) }
func upGrayText(s string) string  { return grayText(s) }
