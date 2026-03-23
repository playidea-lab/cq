package handlers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
)

const errorTraceResourceURI = "ui://cq/error-trace"

// ErrorTraceDeps holds optional dependencies for the error trace handler.
type ErrorTraceDeps struct {
	ResourceStore  *apps.ResourceStore
	ErrorTraceHTML string
}

type errorFrame struct {
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Function string `json:"function,omitempty"`
	Module   string `json:"module,omitempty"`
	IsCause  bool   `json:"is_cause"`
}

type errorTraceResult struct {
	ErrorType string       `json:"error_type"`
	Message   string       `json:"message"`
	Language  string       `json:"language"`
	Frames    []errorFrame `json:"frames"`
}

// RegisterErrorTraceHandler registers the c4_error_trace MCP tool.
func RegisterErrorTraceHandler(reg *mcp.Registry, store *SQLiteStore, deps *ErrorTraceDeps) {
	if deps == nil {
		deps = &ErrorTraceDeps{}
	}

	if deps.ResourceStore != nil && deps.ErrorTraceHTML != "" {
		deps.ResourceStore.Register(errorTraceResourceURI, deps.ErrorTraceHTML)
	}

	reg.Register(mcp.ToolSchema{
		Name:        "c4_error_trace",
		Description: "Parse error stack traces (Go panic, Python traceback, JS Error) into structured frames with cause highlighting",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"trace": map[string]any{
					"type":        "string",
					"description": "Raw error/stack trace text to parse",
				},
				"format": map[string]any{
					"type":        "string",
					"description": "Response format: 'widget' returns MCP Apps widget; 'text' returns plain JSON (default)",
					"enum":        []string{"widget", "text"},
				},
			},
			"required": []string{"trace"},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Trace  string `json:"trace"`
			Format string `json:"format"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		if args.Trace == "" {
			return nil, fmt.Errorf("trace is required")
		}

		result := parseErrorTrace(args.Trace)

		if args.Format == "widget" {
			return map[string]any{
				"data":  result,
				"_meta": map[string]any{"ui": map[string]any{"resourceUri": errorTraceResourceURI}},
			}, nil
		}
		return result, nil
	})
}

func parseErrorTrace(trace string) errorTraceResult {
	trace = strings.TrimSpace(trace)

	if strings.Contains(trace, "goroutine ") || strings.Contains(trace, "panic:") {
		return parseGoTrace(trace)
	}
	if strings.Contains(trace, "Traceback (most recent call last)") || strings.Contains(trace, "File \"") {
		return parsePythonTrace(trace)
	}
	if strings.Contains(trace, "    at ") || strings.Contains(trace, "Error:") {
		return parseJSTrace(trace)
	}

	return errorTraceResult{
		ErrorType: "unknown",
		Message:   trace,
		Language:  "unknown",
	}
}

var goFrameRe = regexp.MustCompile(`^(.+?)\.([^.]+)\(.*\)$`)
var goFileRe = regexp.MustCompile(`^\t(.+):(\d+)`)

func parseGoTrace(trace string) errorTraceResult {
	lines := strings.Split(trace, "\n")
	result := errorTraceResult{Language: "go", ErrorType: "panic"}

	for _, line := range lines {
		if strings.HasPrefix(line, "panic:") {
			result.Message = strings.TrimSpace(strings.TrimPrefix(line, "panic:"))
			break
		}
	}
	if result.Message == "" {
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "goroutine") {
				result.Message = line
				break
			}
		}
	}

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if m := goFrameRe.FindStringSubmatch(line); m != nil {
			frame := errorFrame{Module: m[1], Function: m[2]}
			if i+1 < len(lines) {
				if fm := goFileRe.FindStringSubmatch(lines[i+1]); fm != nil {
					frame.File = fm[1]
					frame.Line, _ = strconv.Atoi(fm[2])
					i++
				}
			}
			result.Frames = append(result.Frames, frame)
		}
	}

	// Mark first non-runtime frame as cause
	for i := range result.Frames {
		if !strings.HasPrefix(result.Frames[i].Module, "runtime") {
			result.Frames[i].IsCause = true
			break
		}
	}

	return result
}

var pyFrameRe = regexp.MustCompile(`File "(.+?)", line (\d+), in (.+)`)

func parsePythonTrace(trace string) errorTraceResult {
	lines := strings.Split(trace, "\n")
	result := errorTraceResult{Language: "python", ErrorType: "Exception"}

	// Last non-empty line is the error message
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			if idx := strings.Index(line, ":"); idx > 0 {
				result.ErrorType = line[:idx]
				result.Message = strings.TrimSpace(line[idx+1:])
			} else {
				result.Message = line
			}
			break
		}
	}

	for _, line := range lines {
		if m := pyFrameRe.FindStringSubmatch(line); m != nil {
			lineNum, _ := strconv.Atoi(m[2])
			result.Frames = append(result.Frames, errorFrame{
				File:     m[1],
				Line:     lineNum,
				Function: m[3],
			})
		}
	}

	// Last frame is the cause in Python
	if len(result.Frames) > 0 {
		result.Frames[len(result.Frames)-1].IsCause = true
	}

	return result
}

var jsFrameRe = regexp.MustCompile(`at\s+(.+?)\s+\((.+?):(\d+)(?::(\d+))?\)`)
var jsFrameRe2 = regexp.MustCompile(`at\s+(.+?):(\d+)(?::(\d+))?$`)

func parseJSTrace(trace string) errorTraceResult {
	lines := strings.Split(trace, "\n")
	result := errorTraceResult{Language: "javascript", ErrorType: "Error"}

	// First line is typically "ErrorType: message"
	if len(lines) > 0 {
		first := strings.TrimSpace(lines[0])
		if idx := strings.Index(first, ":"); idx > 0 {
			result.ErrorType = first[:idx]
			result.Message = strings.TrimSpace(first[idx+1:])
		} else {
			result.Message = first
		}
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if m := jsFrameRe.FindStringSubmatch(line); m != nil {
			lineNum, _ := strconv.Atoi(m[3])
			result.Frames = append(result.Frames, errorFrame{
				Function: m[1],
				File:     m[2],
				Line:     lineNum,
			})
		} else if m := jsFrameRe2.FindStringSubmatch(line); m != nil {
			lineNum, _ := strconv.Atoi(m[2])
			result.Frames = append(result.Frames, errorFrame{
				File: m[1],
				Line: lineNum,
			})
		}
	}

	// First frame is the cause in JS
	if len(result.Frames) > 0 {
		result.Frames[0].IsCause = true
	}

	return result
}
