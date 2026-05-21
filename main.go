package main

import (
	"fmt"
	"os"

	"github.com/Nur-Adnan/duster/cmd"
	"github.com/spf13/cobra"
)

// Version metadata — injected at build time via ldflags:
//
//	go build -ldflags="-X main.Version=1.0.2 -X main.BuildDate=2026-05-20 -X main.Commit=abc1234"
var (
	Version   = "1.0.1"
	BuildDate = "2026-05-20"
	Commit    = "dev"
)

var rootCmd = &cobra.Command{
	Use:   "du",
	Short: "Duster — Deep clean and optimize your Windows PC.",
	Long: `Duster (du) is a powerful, Windows-native command-line cleaning 
and optimization utility designed to reclaim disk space, uninstall software remnants, 
and improve overall system responsiveness.`,
	Version: fmt.Sprintf("%s (built %s, commit %s)", Version, BuildDate, Commit),
	Run: func(cmdCobra *cobra.Command, args []string) {
		cmd.ExecuteLanding()
	},
}

func init() {
	// Wire version into the cmd package so subcommands can reference it
	cmd.AppVersion = Version

	// Configure root version flag
	rootCmd.SetVersionTemplate("Duster version {{.Version}}\n")

	// Wire all subcommands from the cmd package
	rootCmd.AddCommand(cmd.CleanCmd)
	rootCmd.AddCommand(cmd.UninstallCmd)
	rootCmd.AddCommand(cmd.OptimizeCmd)
	rootCmd.AddCommand(cmd.AnalyzeCmd)
	rootCmd.AddCommand(cmd.StatusCmd)
	rootCmd.AddCommand(cmd.PurgeCmd)
	rootCmd.AddCommand(cmd.InstallerCmd)
	rootCmd.AddCommand(cmd.UpdateCmd)
	rootCmd.AddCommand(cmd.RemoveCmd)
	rootCmd.AddCommand(cmd.DoctorCmd)
	rootCmd.AddCommand(cmd.BenchmarkCmd)
	rootCmd.AddCommand(cmd.VerifyCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
