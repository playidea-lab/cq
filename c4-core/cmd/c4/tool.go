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

When "cq serve" is running, calls are routed via Unix socket (~10ms).
Otherwise, the MCP server is initialised inline (~500ms cold start).

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
	// Socket-first: if cq serve is running, avoid a full newMCPServer() init.
	sockPath := toolSockPath()
	var printFn func(w *tabwriter.Writer) int

	if resp, err := callSocket(sockPath, sockRequest{Op: "list"}); err == nil {
		printFn = func(w *tabwriter.Writer) int {
			for _, t := range resp.Tools {
				desc := strings.ReplaceAll(t.Description, "\n", " ")
				if len(desc) > 72 {
					desc = desc[:69] + "..."
				}
				fmt.Fprintf(w, "%s\t%s\n", t.Name, desc)
			}
			return len(resp.Tools)
		}
	} else {
		srv, err := newMCPServer()
		if err != nil {
			return fmt.Errorf("initializing MCP server: %w", err)
		}
		defer srv.shutdown()
		tools := srv.registry.ListTools()
		printFn = func(w *tabwriter.Writer) int {
			for _, t := range tools {
				desc := strings.ReplaceAll(t.Description, "\n", " ")
				if len(desc) > 72 {
					desc = desc[:69] + "..."
				}
				fmt.Fprintf(w, "%s\t%s\n", t.Name, desc)
			}
			return len(tools)
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "TOOL\tDESCRIPTION\n")
	fmt.Fprintf(w, "----\t-----------\n")
	total := printFn(w)
	w.Flush()
	fmt.Fprintf(os.Stdout, "\nTotal: %d tools\n", total)
	return nil
}

// runToolDynamic is the RunE for "cq tool <name> [flags]".
// It builds a dynamic cobra.Command from the named tool's InputSchema and
// executes it — routing via Unix socket when "cq serve" is running, or
// falling back to inline newMCPServer() otherwise.
func runToolDynamic(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	name := args[0]
	remaining := args[1:]

	// "list" is a registered subcommand; forward explicitly because
	// DisableFlagParsing prevents automatic subcommand dispatch.
	if name == "list" {
		return runToolList(cmd, remaining)
	}
	if name == "--help" || name == "-h" {
		return cmd.Help()
	}

	sockPath := toolSockPath()

	// Attempt schema retrieval via socket (fast path).
	schemaResp, sockErr := callSocket(sockPath, sockRequest{Op: "schema", Tool: name})
	useSocket := sockErr == nil && schemaResp.Schema != nil

	// Prepare the schema and (if needed) the inline server.
	var srv *mcpServer
	var inputSchema map[string]any
	var toolDesc string

	if useSocket {
		inputSchema = schemaResp.Schema.InputSchema
		toolDesc = schemaResp.Schema.Description
	} else {
		var err error
		srv, err = newMCPServer()
		if err != nil {
			return fmt.Errorf("initializing MCP server: %w", err)
		}
		defer srv.shutdown()

		schema, ok := srv.registry.GetToolSchema(name)
		if !ok {
			return fmt.Errorf("unknown tool: %q (run 'cq tool list' to see available tools)", name)
		}
		inputSchema = schema.InputSchema
		toolDesc = schema.Description
	}

	// Build a dynamic cobra.Command from the tool's InputSchema.
	dynCmd := &cobra.Command{
		Use:           name,
		Short:         toolDesc,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	dynCmd.Flags().Bool("json", false, "output raw JSON")
	dynCmd.Flags().Duration("timeout", 60*time.Second, "tool call timeout")

	props := extractProperties(inputSchema)
	for propName, propDef := range props {
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
		if useSocket {
			return execToolViaSocket(sockPath, c, name, props)
		}
		return execTool(c, name, srv, props)
	}

	dynCmd.SetArgs(remaining)
	return dynCmd.Execute()
}

// buildToolArgsMap converts parsed cobra flags to the args map expected by
// CallWithContext. Shared between execTool (inline) and execToolViaSocket.
func buildToolArgsMap(cmd *cobra.Command, props map[string]any) (map[string]any, error) {
	argsMap := map[string]any{}
	for propName, propDef := range props {
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
				return nil, fmt.Errorf("flag --%s: %q is not a valid integer: %w", propName, val, err)
			}
			argsMap[propName] = n
		case "number":
			f64, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return nil, fmt.Errorf("flag --%s: %q is not a valid number: %w", propName, val, err)
			}
			argsMap[propName] = f64
		case "array":
			// Try JSON array first (e.g. --tags='["a","b"]'), fall back to CSV.
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
	return argsMap, nil
}

// execTool calls the tool via the inline mcpServer (cold-start path).
func execTool(cmd *cobra.Command, name string, srv *mcpServer, props map[string]any) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	timeout, _ := cmd.Flags().GetDuration("timeout")

	argsMap, err := buildToolArgsMap(cmd, props)
	if err != nil {
		return err
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

// execToolViaSocket calls the tool via the Unix socket (fast path).
func execToolViaSocket(sockPath string, cmd *cobra.Command, name string, props map[string]any) error {
	jsonOut, _ := cmd.Flags().GetBool("json")

	argsMap, err := buildToolArgsMap(cmd, props)
	if err != nil {
		return err
	}

	resp, err := callSocket(sockPath, sockRequest{Op: "call", Tool: name, Args: argsMap})
	if err != nil {
		return fmt.Errorf("socket unavailable (is cq serve running?): %w", err)
	}

	if jsonOut {
		b, err := json.MarshalIndent(resp.Result, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding result: %w", err)
		}
		fmt.Println(string(b))
		return nil
	}
	return prettyPrint(resp.Result)
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
