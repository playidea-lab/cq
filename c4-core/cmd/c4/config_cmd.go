package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/mcp/handlers/cfghandler"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var configGlobal bool

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configSetCmd.Flags().BoolVar(&configGlobal, "global", false, "write to ~/.c4/config.yaml (shared across all projects)")
	rootCmd.AddCommand(configCmd)
}

// configEntry represents a single resolved configuration value.
type configEntry struct {
	Key     string // dot-notation: "cloud.enabled"
	Section string // top-level: "cloud"
	Value   any
	Source  string // "default", "project", "global", "env"
	Kind    string // "bool", "string", "int", "array"
}

// walkStruct recursively walks a struct, emitting configEntry for each leaf field.
func walkStruct(t reflect.Type, v reflect.Value, prefix string, projectYAML, globalYAML string, entries *[]configEntry) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fval := v.Field(i)

		tag := field.Tag.Get("mapstructure")
		if tag == "" || tag == "-" {
			continue
		}
		// strip options like ",omitempty"
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}

		var key string
		if prefix == "" {
			key = tag
		} else {
			key = prefix + "." + tag
		}

		fkind := fval.Kind()

		// dereference pointers
		if fkind == reflect.Ptr {
			if fval.IsNil() {
				continue
			}
			fval = fval.Elem()
			fkind = fval.Kind()
		}

		// skip maps
		if fkind == reflect.Map {
			continue
		}

		if fkind == reflect.Struct {
			walkStruct(fval.Type(), fval, key, projectYAML, globalYAML, entries)
			continue
		}

		// leaf field
		var kind string
		switch fkind {
		case reflect.Bool:
			kind = "bool"
		case reflect.String:
			kind = "string"
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Float32, reflect.Float64, reflect.Uint, reflect.Uint8, reflect.Uint16,
			reflect.Uint32, reflect.Uint64:
			kind = "int"
		case reflect.Slice:
			kind = "array"
		default:
			kind = "string"
		}

		// determine source
		source := "default"
		envKey := "C4_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
		if os.Getenv(envKey) != "" {
			source = "env"
		} else {
			// simple heuristic: check if the leaf key name appears in project/global YAML
			leafKey := tag
			if strings.Contains(projectYAML, leafKey+":") || strings.Contains(projectYAML, leafKey+" :") {
				source = "project"
			} else if strings.Contains(globalYAML, leafKey+":") || strings.Contains(globalYAML, leafKey+" :") {
				source = "global"
			}
		}

		section := key
		if idx := strings.Index(key, "."); idx != -1 {
			section = key[:idx]
		}

		*entries = append(*entries, configEntry{
			Key:     key,
			Section: section,
			Value:   fval.Interface(),
			Source:  source,
			Kind:    kind,
		})
	}
}

// scanConfigEntries loads the config and returns all leaf entries.
func scanConfigEntries(dir string) ([]configEntry, error) {
	mgr, err := config.New(dir)
	if err != nil && mgr == nil {
		return nil, err
	}

	projectYAML, _ := os.ReadFile(filepath.Join(dir, ".c4", "config.yaml"))
	home, _ := os.UserHomeDir()
	globalYAML, _ := os.ReadFile(filepath.Join(home, ".c4", "config.yaml"))

	var entries []configEntry
	cfg := mgr.GetConfig()
	walkStruct(reflect.TypeOf(cfg), reflect.ValueOf(cfg), "", string(projectYAML), string(globalYAML), &entries)

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Section != entries[j].Section {
			return entries[i].Section < entries[j].Section
		}
		return entries[i].Key < entries[j].Key
	})
	return entries, nil
}

// runConfigTUI launches the interactive config TUI.
func runConfigTUI() error {
	// TODO(T-907): implement bubbletea TUI
	entries, err := scanConfigEntries(projectDir)
	if err != nil {
		return err
	}
	fmt.Printf("%d config entries\n", len(entries))
	return nil
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Read or write config values",
	Long: `Read or write configuration values.

Keys use dot-notation (e.g. serve.mcp_http.enabled).

Config layers (highest priority first):
  1. Environment variables (C4_*)
  2. Project config (.c4/config.yaml)
  3. Global config (~/.c4/config.yaml)
  4. Built-in defaults

Examples:
  cq config get serve.mcp_http.port
  cq config set serve.mcp_http.enabled true
  cq config set --global cloud.mode cloud-primary
  cq config set --global permission_reviewer.enabled true`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdout.Fd())) {
			return cmd.Help()
		}
		return runConfigTUI()
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgMgr, err := config.New(projectDir)
		if err != nil && cfgMgr == nil {
			return fmt.Errorf("config load: %w", err)
		}
		fmt.Println(cfgMgr.Get(args[0]))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		var configPath string
		if configGlobal {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("home dir: %w", err)
			}
			globalDir := filepath.Join(home, ".c4")
			if err := os.MkdirAll(globalDir, 0o755); err != nil {
				return fmt.Errorf("create ~/.c4: %w", err)
			}
			configPath = filepath.Join(globalDir, "config.yaml")
		} else {
			configPath = cfghandler.ConfigFilePath(projectDir)
		}

		if err := cfghandler.UpdateYAMLValue(configPath, key, value); err != nil {
			return fmt.Errorf("config set: %w", err)
		}

		scope := "project"
		if configGlobal {
			scope = "global"
		}
		fmt.Fprintf(os.Stderr, "cq: config [%s] %q = %q\n", scope, key, value)
		return nil
	},
}
