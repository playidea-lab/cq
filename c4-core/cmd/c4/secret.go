package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/changmin/c4-core/internal/secrets"
)

func init() {
	secretCmd.AddCommand(secretSetCmd)
	secretCmd.AddCommand(secretGetCmd)
	secretCmd.AddCommand(secretListCmd)
	secretCmd.AddCommand(secretDeleteCmd)
	rootCmd.AddCommand(secretCmd)
}

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage encrypted secrets (~/.c4/secrets.db)",
	Long: `Encrypted secret storage backed by ~/.c4/secrets.db.
Master key is auto-generated at ~/.c4/master.key (0400).
For CI, set C4_MASTER_KEY=<64 hex chars> environment variable.

Key naming convention:
  openai.api_key      LLM Gateway — OpenAI
  anthropic.api_key   LLM Gateway — Anthropic
  gemini.api_key      LLM Gateway — Gemini`,
}

var secretSetCmd = &cobra.Command{
	Use:   "set <key>",
	Short: "Set a secret (prompts for value, not echoed)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		value, err := readHiddenInput(fmt.Sprintf("Value for %q (hidden): ", key))
		if err != nil {
			return fmt.Errorf("read value: %w", err)
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("value must not be empty")
		}

		store, err := secrets.New()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Set(key, value); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "cq: secret %q saved\n", key)
		return nil
	},
}

var secretGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a secret value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := secrets.New()
		if err != nil {
			return err
		}
		defer store.Close()

		val, err := store.Get(args[0])
		if err != nil {
			return err
		}
		fmt.Println(val)
		return nil
	},
}

var secretListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all secret keys (values not shown)",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := secrets.New()
		if err != nil {
			return err
		}
		defer store.Close()

		keys, err := store.List()
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			fmt.Fprintln(os.Stderr, "(no secrets stored)")
			return nil
		}
		for _, k := range keys {
			fmt.Println(k)
		}
		return nil
	},
}

var secretDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a secret",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := secrets.New()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Delete(args[0]); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "cq: secret %q deleted\n", args[0])
		return nil
	},
}

// readHiddenInput reads a line from stdin without echoing characters.
// Falls back to normal stdin read for non-terminal environments (pipes/CI).
func readHiddenInput(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)

	fd := int(os.Stdin.Fd())
	if isTerminal(fd) {
		// Disable echo
		old, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning: could not disable echo; input will be visible")
			return readLine()
		}
		noEcho := *old
		noEcho.Lflag &^= syscall.ECHO
		if err := unix.IoctlSetTermios(fd, ioctlSetTermios, &noEcho); err != nil {
			fmt.Fprintln(os.Stderr, "warning: could not disable echo; input will be visible")
			return readLine()
		}
		defer func() {
			unix.IoctlSetTermios(fd, ioctlSetTermios, old)
			fmt.Fprintln(os.Stderr) // newline after hidden input
		}()
	}
	return readLine()
}

func readLine() (string, error) {
	var line strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			if buf[0] == '\r' {
				continue // strip carriage return (Windows-style line endings)
			}
			line.WriteByte(buf[0])
		}
		if err != nil {
			break
		}
	}
	return line.String(), nil
}

func isTerminal(fd int) bool {
	_, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	return err == nil
}
