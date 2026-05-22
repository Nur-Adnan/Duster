package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// SetupCustomHelp overrides Cobra's default help and usage printer with an extremely high-fidelity,
// beautiful, colorful, modern design system powered by Lipgloss.
func SetupCustomHelp(rootCmd *cobra.Command) {
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printBeautifulHelp(cmd)
	})

	rootCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		printBeautifulHelp(cmd)
		return nil
	})
}

func printBeautifulHelp(cmd *cobra.Command) {
	var sb strings.Builder

	// 1. Render the magnificent high-fidelity ASCII header with duster command prompt
	cmdPath := cmd.CommandPath()
	sb.WriteString(RenderHeader(80, cmdPath))

	// 2. Render Description/Long text
	desc := cmd.Long
	if desc == "" {
		desc = cmd.Short
	}
	desc = strings.TrimSpace(desc)
	if desc != "" {
		sb.WriteString("  " + styleTitle.Render("DESCRIPTION") + "\n")
		// Indent and wrap description paragraphs
		lines := strings.Split(desc, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				sb.WriteString("    " + styleValue.Render(trimmed) + "\n")
			}
		}
		sb.WriteString("\n")
	}

	// 3. Render Usage
	sb.WriteString("  " + styleTitle.Render("USAGE") + "\n")
	
	// Create a styled usage string
	useLine := cmd.UseLine()
	// Highlight "du" and [command]/[flags]
	useLine = strings.Replace(useLine, "du", styleSuccess.Render("du"), 1)
	useLine = strings.Replace(useLine, "[command]", styleAccent.Render("[command]"), -1)
	useLine = strings.Replace(useLine, "[flags]", styleWarning.Render("[flags]"), -1)
	
	sb.WriteString("    " + useLine + "\n\n")

	// 4. Render Available Commands
	commands := cmd.Commands()
	var available []*cobra.Command
	for _, c := range commands {
		if c.IsAvailableCommand() && !c.Hidden {
			available = append(available, c)
		}
	}

	if len(available) > 0 {
		sb.WriteString("  " + styleTitle.Render("AVAILABLE COMMANDS") + "\n")
		maxNameLen := 0
		for _, c := range available {
			if len(c.Name()) > maxNameLen {
				maxNameLen = len(c.Name())
			}
		}

		for _, c := range available {
			bullet := styleSuccess.Render("▪")
			nameStr := padRight(c.Name(), maxNameLen+2)
			nameRendered := styleAccent.Render(nameStr)
			descRendered := styleLabel.Render(c.Short)

			sb.WriteString(fmt.Sprintf("    %s %s %s\n", bullet, nameRendered, descRendered))
		}
		sb.WriteString("\n")
	}

	// 5. Render Local Flags
	localFlags := cmd.LocalFlags()
	var flagsList []*pflag.Flag
	localFlags.VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			flagsList = append(flagsList, f)
		}
	})

	if len(flagsList) > 0 {
		sb.WriteString("  " + styleTitle.Render("FLAGS") + "\n")
		
		var flagNames []string
		maxFlagLen := 0
		for _, f := range flagsList {
			var nameStr string
			if f.Shorthand != "" {
				nameStr = fmt.Sprintf("-%s, --%s", f.Shorthand, f.Name)
			} else {
				nameStr = fmt.Sprintf("    --%s", f.Name)
			}
			if f.Value.Type() != "bool" {
				nameStr += fmt.Sprintf(" <%s>", f.Value.Type())
			}
			flagNames = append(flagNames, nameStr)
			if len(nameStr) > maxFlagLen {
				maxFlagLen = len(nameStr)
			}
		}

		for i, f := range flagsList {
			bullet := styleWarning.Render("○")
			nameStr := padRight(flagNames[i], maxFlagLen+2)
			nameRendered := styleAccent.Render(nameStr)
			descRendered := styleLabel.Render(f.Usage)
			
			defaultStr := ""
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "[]" {
				defaultStr = styleMuted.Render(fmt.Sprintf(" (default: %s)", f.DefValue))
			}

			sb.WriteString(fmt.Sprintf("    %s %s %s%s\n", bullet, nameRendered, descRendered, defaultStr))
		}
		sb.WriteString("\n")
	}

	// 6. Render Inherited Flags (Global Flags)
	inheritedFlags := cmd.InheritedFlags()
	var inhFlagsList []*pflag.Flag
	inheritedFlags.VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			inhFlagsList = append(inhFlagsList, f)
		}
	})

	if len(inhFlagsList) > 0 {
		sb.WriteString("  " + styleTitle.Render("GLOBAL FLAGS") + "\n")
		
		var flagNames []string
		maxFlagLen := 0
		for _, f := range inhFlagsList {
			var nameStr string
			if f.Shorthand != "" {
				nameStr = fmt.Sprintf("-%s, --%s", f.Shorthand, f.Name)
			} else {
				nameStr = fmt.Sprintf("    --%s", f.Name)
			}
			if f.Value.Type() != "bool" {
				nameStr += fmt.Sprintf(" <%s>", f.Value.Type())
			}
			flagNames = append(flagNames, nameStr)
			if len(nameStr) > maxFlagLen {
				maxFlagLen = len(nameStr)
			}
		}

		for i, f := range inhFlagsList {
			bullet := styleWarning.Render("○")
			nameStr := padRight(flagNames[i], maxFlagLen+2)
			nameRendered := styleAccent.Render(nameStr)
			descRendered := styleLabel.Render(f.Usage)

			defaultStr := ""
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "[]" {
				defaultStr = styleMuted.Render(fmt.Sprintf(" (default: %s)", f.DefValue))
			}

			sb.WriteString(fmt.Sprintf("    %s %s %s%s\n", bullet, nameRendered, descRendered, defaultStr))
		}
		sb.WriteString("\n")
	}

	// 7. Render Learn More Footer
	sb.WriteString("  " + styleMuted.Render("Use ") +
		styleSuccess.Render("du [command] --help") +
		styleMuted.Render(" for more information about a command.") + "\n\n")

	fmt.Print(sb.String())
}
