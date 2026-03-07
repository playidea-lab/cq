package main

import (
	"fmt"
	"io"
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
	Use:   "set <key> [value]",
	Short: "Set a secret (prompts for value if not provided)",
	Long: `Set an encrypted secret. If value is not provided as an argument,
it is read interactively (hidden, not echoed to terminal).

Providing value as an argument is convenient for scripting but will
appear in shell history — use the interactive prompt for sensitive values.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		store, err := secrets.New()
		if err != nil {
			return err
		}
		defer store.Close()

		var value string
		if len(args) == 2 {
			// Value provided as argument — warn about shell history.
			value = args[1]
			fmt.Fprintln(os.Stderr, "cq: warning: secret value passed as argument will appear in shell history")
		} else {
			value, err = readHiddenInput(fmt.Sprintf("Value for %q (hidden): ", key))
			if err != nil {
				return fmt.Errorf("read value: %w", err)
			}
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("value must not be empty")
		}

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
				return line.String(), nil
			}
			if buf[0] == '\r' {
				continue // strip carriage return (Windows-style line endings)
			}
			line.WriteByte(buf[0])
		}
		if err != nil {
			if err == io.EOF {
				return line.String(), nil // partial or empty line on EOF is ok
			}
			return line.String(), err // propagate real I/O errors
		}
	}
}

func isTerminal(fd int) bool {
	_, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	return err == nil
}
