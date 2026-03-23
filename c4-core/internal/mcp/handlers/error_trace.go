package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
)

const errorTraceResourceURI = "ui://cq/error-trace"

// ErrorTraceDeps holds dependencies for the error trace handler.
type ErrorTraceDeps struct {
	ResourceStore  *apps.ResourceStore
	ErrorTraceHTML string
}

// RegisterErrorTraceHandler registers the c4_error_trace MCP tool and, if deps.ResourceStore
// is non-nil, registers the error trace HTML at ui://cq/error-trace.
func RegisterErrorTraceHandler(reg *mcp.Registry, store *SQLiteStore, deps *ErrorTraceDeps) {
	if deps == nil {
		deps = &ErrorTraceDeps{}
	}

	if deps.ResourceStore != nil && deps.ErrorTraceHTML != "" {
		deps.ResourceStore.Register(errorTraceResourceURI, deps.ErrorTraceHTML)
	}

	// Best-effort lighthouse registration.
	if store != nil {
		if err := store.promoteLighthouse("c4_error_trace", "auto-register"); err != nil {
			if _, err2 := lighthouseRegisterExisting(store, "c4_error_trace",
				"Parse error stack trace text (Go panic, Python traceback, JS Error) into structured frames with cause highlighting",
				`{"type":"object","properties":{"text":{"type":"string"},"format":{"type":"string","enum":["widget","text"]}}}`,
				"Returns structured frames array + MCP Apps widget (_meta.ui.resourceUri=ui://cq/error-trace) when format=widget.",
				"auto-register",
			); err2 != nil {
				fmt.Fprintf(os.Stderr, "c4_error_trace: lighthouse register: %v\n", err2)
			}
		}
	}

	reg.Register(mcp.ToolSchema{
		Name:        "c4_error_trace",
		Description: "Parse error stack trace text (Go panic, Python traceback, JS Error) into structured frames with cause highlighting",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "Raw error/traceback text to parse",
				},
				"format": map[string]any{
					"type":        "string",
					"description": "Response format: 'widget' returns MCP Apps widget with _meta; 'text' returns plain JSON (default)",
					"enum":        []string{"widget", "text"},
				},
			},
			"required": []string{"text"},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Text   string `json:"text"`
			Format string `json:"format"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		if strings.TrimSpace(args.Text) == "" {
			return nil, fmt.Errorf("text is required")
		}

		data := parseErrorTrace(args.Text)

		if args.Format == "widget" {
			return map[string]any{
				"data": data,
				"_meta": map[string]any{
					"ui": map[string]any{
						"resourceUri": errorTraceResourceURI,
					},
				},
			}, nil
		}
		return data, nil
	})
}

// errorFrame represents a single stack frame extracted from a traceback.
type errorFrame struct {
	Function string `json:"function"`
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Module   string `json:"module,omitempty"`
	IsCause  bool   `json:"is_cause"`
}

// traceResult is the structured output of parseErrorTrace.
type traceResult struct {
	ErrorType    string       `json:"error_type"`
	ErrorMessage string       `json:"error_message"`
	Language     string       `json:"language"`
	Frames       []errorFrame `json:"frames"`
}

// parseErrorTrace detects the format (Go/Python/JS) and returns structured data.
func parseErrorTrace(text string) traceResult {
	text = strings.TrimSpace(text)

	switch {
	case isPythonTraceback(text):
		return parsePythonTraceback(text)
	case isGoTraceback(text):
		return parseGoTraceback(text)
	default:
		return parseJSError(text)
	}
}

func isPythonTraceback(text string) bool {
	return strings.Contains(text, "Traceback (most recent call last)") ||
		strings.Contains(text, "  File \"") && strings.Contains(text, ", line ")
}

func isGoTraceback(text string) bool {
	return strings.Contains(text, "goroutine ") ||
		strings.Contains(text, "panic:") ||
		(strings.Contains(text, ".go:") && strings.Contains(text, "+0x"))
}

// parsePythonTraceback handles Python traceback format:
//
//	Traceback (most recent call last):
//	  File "foo.py", line 10, in bar
//	    code here
//	SomeError: message
func parsePythonTraceback(text string) traceResult {
	lines := strings.Split(text, "\n")
	result := traceResult{Language: "Python"}

	var frames []errorFrame
	var i int
	for i = 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "File \"") {
			// File "path/to/file.py", line N, in func_name
			f := errorFrame{}
			rest := strings.TrimPrefix(line, "File \"")
			if idx := strings.Index(rest, "\""); idx >= 0 {
				f.File = rest[:idx]
				rest = rest[idx+1:]
			}
			// ", line N, in func"
			if idx := strings.Index(rest, "line "); idx >= 0 {
				rest = rest[idx+5:]
				parts := strings.SplitN(rest, ",", 2)
				fmt.Sscanf(strings.TrimSpace(parts[0]), "%d", &f.Line)
				if len(parts) > 1 {
					funcPart := strings.TrimSpace(parts[1])
					funcPart = strings.TrimPrefix(funcPart, "in ")
					f.Function = funcPart
				}
			}
			frames = append(frames, f)
		} else if len(line) > 0 && !strings.HasPrefix(line, "Traceback") && !strings.HasPrefix(line, "#") {
			// Check if this is the final error line (ErrorType: message)
			if idx := strings.Index(line, ": "); idx > 0 {
				candidate := line[:idx]
				if !strings.Contains(candidate, " ") && !strings.HasPrefix(candidate, "File") {
					result.ErrorType = candidate
					result.ErrorMessage = strings.TrimSpace(line[idx+2:])
				}
			}
		}
	}

	if result.ErrorType == "" {
		result.ErrorType = "Exception"
	}

	// Mark last frame as cause (innermost)
	if len(frames) > 0 {
		frames[len(frames)-1].IsCause = true
	}
	result.Frames = frames
	return result
}

// parseGoTraceback handles Go panic/runtime traces.
func parseGoTraceback(text string) traceResult {
	lines := strings.Split(text, "\n")
	result := traceResult{Language: "Go", ErrorType: "panic"}

	// Extract panic message
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "panic:") {
			result.ErrorMessage = strings.TrimSpace(strings.TrimPrefix(line, "panic:"))
			break
		}
	}

	if result.ErrorMessage == "" {
		// Use first non-empty, non-goroutine line
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "goroutine") && !strings.HasPrefix(line, "created by") {
				result.ErrorMessage = line
				break
			}
		}
	}

	var frames []errorFrame
	// Parse goroutine frames: alternating func line / file line
	for i := 0; i < len(lines)-1; i++ {
		line := strings.TrimSpace(lines[i])
		next := strings.TrimSpace(lines[i+1])

		// func line ends with (...) or similar; next line is file:line +0x...
		if strings.HasPrefix(next, "/") || strings.Contains(next, ".go:") {
			f := errorFrame{}
			// Function name
			if idx := strings.LastIndex(line, "("); idx > 0 {
				f.Function = line[:idx]
			} else {
				f.Function = line
			}
			// Extract module from function path
			if idx := strings.LastIndex(f.Function, "/"); idx >= 0 {
				pkg := f.Function[:idx]
				shortFunc := f.Function[idx+1:]
				if dotIdx := strings.Index(shortFunc, "."); dotIdx >= 0 {
					f.Module = pkg + "/" + shortFunc[:dotIdx]
					f.Function = shortFunc[dotIdx+1:]
				}
			}
			// File:line from next line
			filePart := next
			if idx := strings.Index(filePart, " "); idx > 0 {
				filePart = filePart[:idx]
			}
			if idx := strings.LastIndex(filePart, ":"); idx > 0 {
				f.File = filePart[:idx]
				fmt.Sscanf(filePart[idx+1:], "%d", &f.Line)
			} else {
				f.File = filePart
			}
			frames = append(frames, f)
			i++ // skip the file line
		}
	}

	// Mark the first user-code frame (non-runtime) as cause
	causeSet := false
	for i := range frames {
		if !strings.Contains(frames[i].File, "runtime/") && !strings.Contains(frames[i].Function, "runtime.") {
			frames[i].IsCause = true
			causeSet = true
			break
		}
	}
	if !causeSet && len(frames) > 0 {
		frames[0].IsCause = true
	}

	result.Frames = frames
	return result
}

// parseJSError handles JavaScript Error stack traces:
//
//	TypeError: cannot read property 'x' of undefined
//	    at foo (file.js:10:5)
//	    at bar (file.js:20:3)
func parseJSError(text string) traceResult {
	lines := strings.Split(text, "\n")
	result := traceResult{Language: "JavaScript"}

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if i == 0 {
			if idx := strings.Index(line, ": "); idx > 0 {
				result.ErrorType = line[:idx]
				result.ErrorMessage = strings.TrimSpace(line[idx+2:])
			} else {
				result.ErrorType = "Error"
				result.ErrorMessage = line
			}
			continue
		}

		if !strings.HasPrefix(line, "at ") {
			continue
		}

		f := errorFrame{}
		rest := strings.TrimPrefix(line, "at ")

		// "funcName (file:line:col)" or "file:line:col"
		if idx := strings.Index(rest, " ("); idx > 0 {
			f.Function = rest[:idx]
			location := strings.TrimSuffix(rest[idx+2:], ")")
			parseJSLocation(location, &f)
		} else {
			f.Function = "<anonymous>"
			parseJSLocation(rest, &f)
		}

		result.Frames = append(result.Frames, f)
	}

	// Mark first frame as cause (outermost in JS stacks)
	if len(result.Frames) > 0 {
		result.Frames[0].IsCause = true
	}
	return result
}

func parseJSLocation(loc string, f *errorFrame) {
	// loc: file.js:line:col or file.js:line
	parts := strings.Split(loc, ":")
	switch len(parts) {
	case 1:
		f.File = parts[0]
	case 2:
		f.File = parts[0]
		fmt.Sscanf(parts[1], "%d", &f.Line)
	default:
		// Windows path may have drive letter: handle by joining first two if short
		if len(parts[0]) == 1 {
			f.File = parts[0] + ":" + parts[1]
			if len(parts) >= 3 {
				fmt.Sscanf(parts[2], "%d", &f.Line)
			}
		} else {
			f.File = parts[0]
			fmt.Sscanf(parts[1], "%d", &f.Line)
		}
	}
}
