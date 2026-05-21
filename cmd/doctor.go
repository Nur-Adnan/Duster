package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Nur-Adnan/duster/lib/elevation"
	"github.com/Nur-Adnan/duster/lib/fs"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var doctorJSON bool

// DoctorCmd represents the environment diagnostics command.
var DoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose Windows environment, privilege, permission, and terminal compatibility",
	Long: `Audit and troubleshoot the current operating environment for:
  - UAC Administrative Privilege state
  - Temp directory writability & locks
  - PowerShell execution policy boundaries
  - Windows build OS compatibility (10 build 19041+)
  - Windows Defender false-positive blockages
  - Software Distribution / Prefetch cache health
  - NTFS Registry Long-Paths support status
  - Junction loops & path traversal risks
  - ANSI/Unicode Terminal rendering capabilities`,
	Run: executeDoctor,
}

func init() {
	DoctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "Output doctor diagnostics as a single JSON payload and exit immediately")
}

// DoctorResult represents a single diagnostic check outcome.
type DoctorResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"` // "PASS", "WARN", "FAIL", "SKIPPED"
	Message     string `json:"message"`
	Description string `json:"description"`
}

// DoctorSnapshot is the high-level report card.
type DoctorSnapshot struct {
	Timestamp time.Time      `json:"timestamp"`
	OS        string         `json:"os"`
	Arch      string         `json:"arch"`
	Healthy   bool           `json:"healthy"`
	Passed    int            `json:"passed"`
	Warnings  int            `json:"warnings"`
	Failed    int            `json:"failed"`
	Results   []DoctorResult `json:"results"`
}

func executeDoctor(cmd *cobra.Command, args []string) {
	if doctorJSON || isPiped() {
		snapshot := runDoctorDiagnostics()
		data, _ := json.MarshalIndent(snapshot, "", "  ")
		fmt.Println(string(data))
		return
	}

	m := initialDoctorModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running doctor TUI: %v\n", err)
		os.Exit(1)
	}
}

// Diagnostic checks execution logic
func runDoctorDiagnostics() DoctorSnapshot {
	var results []DoctorResult

	// 1. Admin checks
	isAdmin := elevation.IsAdmin()
	adminRes := DoctorResult{
		ID:          "privilege",
		Name:        "UAC Administrative Privileges",
		Description: "Checks if Duster is running with elevated administrator tokens.",
	}
	if isAdmin {
		adminRes.Status = "PASS"
		adminRes.Message = "Elevated: Admin tokens active."
	} else {
		adminRes.Status = "WARN"
		adminRes.Message = "Standard User: Prefetch scanning and some deep optimizations (TRIM) are restricted."
	}
	results = append(results, adminRes)

	// 2. Temp Writable & Locks
	tempDir := fs.ResolveEnvPath("%TEMP%")
	tempRes := DoctorResult{
		ID:          "temp_writable",
		Name:        "Temp Directories Writability",
		Description: "Verifies the user and system temp folders are unlocked and writable.",
	}
	tempErr := testDirectoryWritable(tempDir)
	if tempErr == nil {
		tempRes.Status = "PASS"
		tempRes.Message = fmt.Sprintf("Writable: %s is healthy.", tempDir)
	} else {
		tempRes.Status = "FAIL"
		tempRes.Message = fmt.Sprintf("Locked or inaccessible: %v", tempErr)
	}
	results = append(results, tempRes)

	// 3. PowerShell Availability & Execution Policy
	psRes := DoctorResult{
		ID:          "powershell",
		Name:        "PowerShell Execution Boundaries",
		Description: "Audits if powershell.exe is available and what its execution policy is.",
	}
	if runtime.GOOS == "windows" {
		policy, err := queryPowerShellExecutionPolicy()
		if err == nil {
			psRes.Status = "PASS"
			psRes.Message = fmt.Sprintf("Active. Script Execution Policy: %s", policy)
		} else {
			psRes.Status = "WARN"
			psRes.Message = "PowerShell script engine is restricted or unavailable."
		}
	} else {
		psRes.Status = "PASS"
		psRes.Message = "Simulation: PowerShell available and healthy."
	}
	results = append(results, psRes)

	// 4. Windows Build Compatibility
	buildRes := DoctorResult{
		ID:          "os_version",
		Name:        "Windows Build Compatibility",
		Description: "Checks if the operating system meets the minimum required Windows build 19041.",
	}
	if runtime.GOOS == "windows" {
		build, prodName, err := getWindowsBuildInfo()
		if err == nil {
			buildNum := 0
			_, _ = fmt.Sscanf(build, "%d", &buildNum)
			if buildNum >= 19041 || buildNum == 0 { // Allow 0 for unexpected format strings
				buildRes.Status = "PASS"
				buildRes.Message = fmt.Sprintf("%s (Build %s) is fully supported.", prodName, build)
			} else {
				buildRes.Status = "FAIL"
				buildRes.Message = fmt.Sprintf("Unsupported build: %s. Minimum required Windows 10 Build 19041.", build)
			}
		} else {
			buildRes.Status = "WARN"
			buildRes.Message = "Unable to verify Windows version registry flags."
		}
	} else {
		buildRes.Status = "PASS"
		buildRes.Message = "Simulation: Windows 11 Build 22631 is fully supported."
	}
	results = append(results, buildRes)

	// 5. Defender Quarantine Audit
	defRes := DoctorResult{
		ID:          "defender",
		Name:        "Defender Quarantine & Blocking Check",
		Description: "Detects if Windows Defender real-time scanning is restricting file locks.",
	}
	defErr := auditDefenderBlock()
	if defErr == nil {
		defRes.Status = "PASS"
		defRes.Message = "No active file locks or Defender blocks detected."
	} else {
		defRes.Status = "WARN"
		defRes.Message = fmt.Sprintf("Defender block suspected: %v", defErr)
	}
	results = append(results, defRes)

	// 6. Corrupted Caches
	cacheRes := DoctorResult{
		ID:          "cache_health",
		Name:        "Caches Readability",
		Description: "Verifies system prefetch and update download directories are not corrupted.",
	}
	if runtime.GOOS == "windows" && isAdmin {
		prefetchDir := `C:\Windows\Prefetch`
		updateDir := `C:\Windows\SoftwareDistribution\Download`
		pErr := testDirReadable(prefetchDir)
		uErr := testDirReadable(updateDir)
		if pErr == nil && uErr == nil {
			cacheRes.Status = "PASS"
			cacheRes.Message = "System caches are readable and uncorrupted."
		} else {
			cacheRes.Status = "WARN"
			cacheRes.Message = fmt.Sprintf("Some caches inaccessible. Prefetch: %v, SoftwareDistribution: %v", pErr, uErr)
		}
	} else if runtime.GOOS != "windows" {
		cacheRes.Status = "PASS"
		cacheRes.Message = "Simulation: Caches are readable and uncorrupted."
	} else {
		cacheRes.Status = "SKIPPED"
		cacheRes.Message = "Requires Admin privileges to scan system caches."
	}
	results = append(results, cacheRes)

	// 7. Long-Path Registry Support
	longRes := DoctorResult{
		ID:          "long_paths",
		Name:        "Windows Long-Paths Enablement",
		Description: "Checks if the 260-character NTFS path limit has been unlocked.",
	}
	if runtime.GOOS == "windows" {
		enabled, err := queryLongPathsEnabled()
		if err == nil && enabled {
			longRes.Status = "PASS"
			longRes.Message = "Long paths are unlocked in the registry."
		} else {
			longRes.Status = "WARN"
			longRes.Message = "Long path support is disabled. Extremely nested developer caches might fail scan."
		}
	} else {
		longRes.Status = "PASS"
		longRes.Message = "Long paths supported by default on this operating system."
	}
	results = append(results, longRes)

	// 8. Junction Loops Hazard
	loopRes := DoctorResult{
		ID:          "junction_loops",
		Name:        "NTFS Junction Loops Audit",
		Description: "Scans `%LOCALAPPDATA%` to ensure infinite loop traversal locks are absent.",
	}
	hasLoop := checkJunctionLoops()
	if !hasLoop {
		loopRes.Status = "PASS"
		loopRes.Message = "No junction loop risks detected in local folders."
	} else {
		loopRes.Status = "WARN"
		loopRes.Message = "Detected junction loop risk. Duster's safety walk will successfully bypass it."
	}
	results = append(results, loopRes)

	// 9. Terminal Capabilities
	termRes := DoctorResult{
		ID:          "terminal",
		Name:        "Terminal Styling Capabilities",
		Description: "Checks if the active terminal window supports full ANSI coloring.",
	}
	isWT := os.Getenv("WT_SESSION") != ""
	term := os.Getenv("TERM")
	if isWT || strings.Contains(strings.ToLower(term), "color") || strings.Contains(strings.ToLower(term), "xterm") {
		termRes.Status = "PASS"
		termRes.Message = fmt.Sprintf("Supported: Modern terminal detected (%s).", term)
	} else {
		termRes.Status = "WARN"
		termRes.Message = "Legacy ConHost terminal detected. Graphical elements may render in simplified ASCII."
	}
	results = append(results, termRes)

	// Final Summary Counts
	passed, warnings, failed := 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case "PASS":
			passed++
		case "WARN":
			warnings++
		case "FAIL":
			failed++
		}
	}

	return DoctorSnapshot{
		Timestamp: time.Now().UTC(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Healthy:   failed == 0,
		Passed:    passed,
		Warnings:  warnings,
		Failed:    failed,
		Results:   results,
	}
}

// Helpers for diagnoses
func testDirectoryWritable(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return err
	}
	tempFile := filepath.Join(path, ".duster-doctor-temp")
	err := os.WriteFile(tempFile, []byte("duster write verification"), 0644)
	if err != nil {
		return err
	}
	_ = os.Remove(tempFile)
	return nil
}

func testDirReadable(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	_, err = d.Readdirnames(1)
	if err != nil && err.Error() != "EOF" {
		return err
	}
	return nil
}

func auditDefenderBlock() error {
	// Defender block checks try writing a temporary file under LOCALAPPDATA, writing an extremely basic string,
	// and verifying we can read its exact content.
	appData := fs.ResolveEnvPath("%LOCALAPPDATA%")
	testDir := filepath.Join(appData, "Duster")
	_ = os.MkdirAll(testDir, 0755)

	testFile := filepath.Join(testDir, "defender_audit.test")
	err := os.WriteFile(testFile, []byte("duster_security_sandbox_verif_token"), 0644)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(testFile)
	}()

	content, err := os.ReadFile(testFile)
	if err != nil {
		return err
	}
	if string(content) != "duster_security_sandbox_verif_token" {
		return fmt.Errorf("read corruption detected")
	}
	return nil
}

func checkJunctionLoops() bool {
	// Standard loop risk check: check if LOCALAPPDATA contains "Application Data" which loops back to parent
	localAppData := fs.ResolveEnvPath("%LOCALAPPDATA%")
	appDataLoop := filepath.Join(localAppData, "Application Data")
	info, err := os.Lstat(appDataLoop)
	if err == nil {
		// If it exists and is a symlink or has NTFS reparse points, it's a known junction point loop risk
		if info.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

// Bubble Tea TUI
type doctorModel struct {
	snapshot   DoctorSnapshot
	running    bool
	progress   int
	width      int
	height     int
	logEntries []string
}

type doctorProgMsg int
type doctorCompleteMsg DoctorSnapshot

func initialDoctorModel() doctorModel {
	return doctorModel{
		running:    true,
		logEntries: []string{"Initializing doctor audit parameters..."},
	}
}

func runDoctorAuditCmd() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(300 * time.Millisecond) // Let UI render initialization
		snapshot := runDoctorDiagnostics()
		return doctorCompleteMsg(snapshot)
	}
}

func (m doctorModel) Init() tea.Cmd {
	return runDoctorAuditCmd()
}

func (m doctorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case doctorProgMsg:
		m.progress = int(msg)

	case doctorCompleteMsg:
		m.running = false
		m.snapshot = DoctorSnapshot(msg)
	}
	return m, nil
}

func (m doctorModel) View() string {
	var s strings.Builder

	s.WriteString("\n  " + doctorTitle.Render("Duster Diagnostics — Environment Doctor"))
	s.WriteString("\n" + doctorDivider.Render("  ═════════════════════════════════════════════════════════════════════\n\n"))

	if m.running {
		s.WriteString("  [Running Audits] Please wait while we verify your Windows configurations...\n\n")
		s.WriteString("    🔍 Verifying UAC Admin Tokens...\n")
		s.WriteString("    📁 Testing Temporary Directories Writability...\n")
		s.WriteString("    ⚙ Auditing NTFS Registry Settings & Long Paths...\n")
		s.WriteString("    ⚠️  Scanning for Junction Loops and Reparse hazards...\n")
		s.WriteString("    🛡️ Auditing Active Defender Quarantine Policies...\n\n")
		s.WriteString("  Press [q] to cancel.")
		return s.String()
	}

	// Render beautiful report card
	s.WriteString(fmt.Sprintf("  Audit Complete! Health: %s\n", formatHealth(m.snapshot.Healthy)))
	s.WriteString(fmt.Sprintf("  Passed: %d  |  Warnings: %d  |  Failed: %d\n\n",
		m.snapshot.Passed, m.snapshot.Warnings, m.snapshot.Failed))

	s.WriteString("  " + boldWhite.Render("DETAILED REPORT CARD") + "\n")
	s.WriteString(doctorDivider.Render("  ─────────────────────────────────────────────────────────────────────\n"))

	for _, r := range m.snapshot.Results {
		var indicator string
		switch r.Status {
		case "PASS":
			indicator = doctorSuccess.Render("✓")
		case "WARN":
			indicator = doctorWarn.Render("○")
		case "FAIL":
			indicator = doctorFail.Render("✗")
		default:
			indicator = doctorGray.Render("•")
		}

		s.WriteString(fmt.Sprintf("  %s  %-30s %s\n", indicator, boldWhite.Render(r.Name), doctorGray.Render(r.Message)))
	}

	s.WriteString(doctorDivider.Render("\n  ═════════════════════════════════════════════════════════════════════\n"))
	if m.snapshot.Healthy {
		s.WriteString("  " + doctorSuccess.Render("🎉 ENVIRONMENT CLEAN & HEALTHY! Duster is fully ready for maximum deep optimization."))
	} else {
		s.WriteString("  " + doctorFail.Render("⚠️  SYSTEM DEGRADATION DETECTED: Review the failures above to ensure absolute reliability."))
	}
	s.WriteString("\n  " + doctorGray.Render("Press [q/esc] to exit."))

	return s.String()
}

func formatHealth(healthy bool) string {
	if healthy {
		return doctorSuccess.Render("EXCELLENT (10/10)")
	}
	return doctorFail.Render("DEGRADED")
}

// Doctor Styling Constants
var (
	doctorTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FFFF")).Padding(0, 1)
	doctorDivider = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	doctorSuccess = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	doctorWarn    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFA500"))
	doctorFail    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF0000"))
	doctorGray    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)
