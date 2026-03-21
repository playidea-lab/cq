package main

import (
	"strings"

	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/spf13/cobra"
)

// collectCLICommands traverses the Cobra command tree and returns a flat list
// of CLI commands suitable for lighthouse registration. Hidden commands and
// completion/help builtins are skipped.
func collectCLICommands(root *cobra.Command, prefix string) []handlers.CLICommand {
	var cmds []handlers.CLICommand
	walkCommands(root, prefix, &cmds)
	return cmds
}

func walkCommands(cmd *cobra.Command, prefix string, out *[]handlers.CLICommand) {
	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		name := child.Name()
		if name == "help" || name == "completion" {
			continue
		}

		fullCmd := prefix + " " + name
		short := child.Short
		long := child.Long
		if long == "" {
			long = short
		}
		// Trim long descriptions to keep lighthouse entries concise.
		if len(long) > 300 {
			long = long[:300] + "..."
		}

		// Only register leaf commands (commands with RunE/Run set).
		if child.RunE != nil || child.Run != nil {
			*out = append(*out, handlers.CLICommand{
				FullCommand: fullCmd,
				Short:       short,
				Long:        strings.TrimSpace(long),
			})
		}

		// Recurse into subcommands.
		if child.HasSubCommands() {
			walkCommands(child, fullCmd, out)
		}
	}
}
