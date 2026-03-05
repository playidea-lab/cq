package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var toolCmd = &cobra.Command{
	Use:   "tool",
	Short: "MCP→CLI auto-gateway: call any MCP tool from the command line",
	Long: `cq tool exposes all registered MCP tools as CLI commands.

Agents can call read-only tools via Bash("cq tool <name> --json")
instead of registering them as MCP tools — reducing schema token overhead.

Examples:
  cq tool list
  cq tool c4_status --json
  cq tool c4_task_list --include_dod=false --json
  cq tool c4_knowledge_search --query="패턴" --json
  cq tool c4_find_file --pattern="tool.go" --json`,
	// DisableFlagParsing=true makes cobra pass ALL args (including --flags)
	// as positional args to RunE, so we can forward them to the dynamically
	// built sub-command for the named tool.
	RunE:               runToolDynamic,
	DisableFlagParsing: true,
}

var toolListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available MCP tools",
	RunE:  runToolList,
}

func init() {
	toolCmd.AddCommand(toolListCmd)
	rootCmd.AddCommand(toolCmd)
}

func runToolList(_ *cobra.Command, _ []string) error {
	srv, err := newMCPServer()
	if err != nil {
		return fmt.Errorf("initializing MCP server: %w", err)
	}
	defer srv.shutdown()

	tools := srv.registry.ListTools()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "TOOL\tDESCRIPTION\n")
	fmt.Fprintf(w, "----\t-----------\n")
	for _, t := range tools {
		desc := strings.ReplaceAll(t.Description, "\n", " ")
		if len(desc) > 72 {
			desc = desc[:69] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\n", t.Name, desc)
	}
	w.Flush()
	fmt.Fprintf(os.Stdout, "\nTotal: %d tools\n", len(tools))
	return nil
}

// runToolDynamic is the RunE for "cq tool <name> [flags]".
// It initializes the MCP server, builds a dynamic cobra.Command for the named
// tool from its InputSchema, and executes it with the remaining args.
func runToolDynamic(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	name := args[0]
	remaining := args[1:]

	// "list" is a registered subcommand; forward to it explicitly because
	// DisableFlagParsing prevents automatic subcommand dispatch.
	if name == "list" {
		return runToolList(cmd, remaining)
	}
	// Handle "--help" / "-h" at the tool level
	if name == "--help" || name == "-h" {
		return cmd.Help()
	}

	srv, err := newMCPServer()
	if err != nil {
		return fmt.Errorf("initializing MCP server: %w", err)
	}
	defer srv.shutdown()

	schema, ok := srv.registry.GetToolSchema(name)
	if !ok {
		return fmt.Errorf("unknown tool: %q (run 'cq tool list' to see available tools)", name)
	}

	dynCmd := &cobra.Command{
		Use:          name,
		Short:        schema.Description,
		SilenceUsage: true,
		SilenceErrors: true,
	}

	// --json flag (reserved; skip if tool schema has a "json" property)
	dynCmd.Flags().Bool("json", false, "output raw JSON")

	// --timeout flag: default 60s to prevent indefinite hang
	dynCmd.Flags().Duration("timeout", 60*time.Second, "tool call timeout")

	// Generate flags from InputSchema properties
	props := extractProperties(schema.InputSchema)
	for propName, propDef := range props {
		// Skip reserved flag names
		if propName == "json" || propName == "timeout" {
			continue
		}
		propMap, ok := propDef.(map[string]any)
		if !ok {
			dynCmd.Flags().String(propName, "", "")
			continue
		}
		propDesc, _ := propMap["description"].(string)
		dynCmd.Flags().String(propName, "", propDesc)
	}

	dynCmd.RunE = func(c *cobra.Command, _ []string) error {
		return execTool(c, name, srv, props)
	}

	dynCmd.SetArgs(remaining)
	return dynCmd.Execute()
}

// execTool builds the args JSON from parsed flags and calls the tool.
func execTool(cmd *cobra.Command, name string, srv *mcpServer, props map[string]any) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	timeout, _ := cmd.Flags().GetDuration("timeout")

	argsMap := map[string]any{}
	for propName, propDef := range props {
		// Skip reserved flags
		if propName == "json" || propName == "timeout" {
			continue
		}
		f := cmd.Flags().Lookup(propName)
		if f == nil || !f.Changed {
			continue
		}
		val := f.Value.String()

		propType := ""
		if propMap, ok := propDef.(map[string]any); ok {
			propType, _ = propMap["type"].(string)
		}

		switch propType {
		case "boolean":
			argsMap[propName] = val == "true" || val == "1" || val == "yes"
		case "integer":
			n, err := strconv.ParseInt(val, 0, 64)
			if err != nil {
				return fmt.Errorf("flag --%s: %q is not a valid integer: %w", propName, val, err)
			}
			argsMap[propName] = n
		case "number":
			f64, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return fmt.Errorf("flag --%s: %q is not a valid number: %w", propName, val, err)
			}
			argsMap[propName] = f64
		case "array":
			// Try JSON array first (e.g. --tags='["a","b"]'), fall back to CSV
			var arr []any
			if err := json.Unmarshal([]byte(val), &arr); err == nil {
				argsMap[propName] = arr
			} else {
				argsMap[propName] = strings.Split(val, ",")
			}
		case "object":
			var obj any
			if err := json.Unmarshal([]byte(val), &obj); err == nil {
				argsMap[propName] = obj
			} else {
				argsMap[propName] = val
			}
		default:
			argsMap[propName] = val
		}
	}

	argsJSON, err := json.Marshal(argsMap)
	if err != nil {
		return fmt.Errorf("marshaling args: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := srv.registry.CallWithContext(ctx, name, argsJSON)
	if err != nil {
		return fmt.Errorf("tool error: %w", err)
	}

	if jsonOut {
		b, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding result: %w", err)
		}
		fmt.Println(string(b))
		return nil
	}

	return prettyPrint(result)
}

// extractProperties returns the "properties" map from an MCP InputSchema.
func extractProperties(inputSchema map[string]any) map[string]any {
	if inputSchema == nil {
		return nil
	}
	props, _ := inputSchema["properties"].(map[string]any)
	return props
}

// prettyPrint formats a tool result for human consumption.
func prettyPrint(result any) error {
	switch v := result.(type) {
	case string:
		fmt.Println(v)
	case []any:
		// MCP text-content array: [{type:"text", text:"..."}]
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok {
					fmt.Print(text)
					continue
				}
			}
			b, err := json.MarshalIndent(item, "", "  ")
			if err != nil {
				return fmt.Errorf("encoding result item: %w", err)
			}
			fmt.Println(string(b))
		}
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding result: %w", err)
		}
		fmt.Println(string(b))
	}
	return nil
}
