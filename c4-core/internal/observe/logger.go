// Package observe provides structured logging, metrics, and middleware
// for observability of MCP tool calls in the C4 engine.
package observe

import (
	"io"
	"log/slog"
	"os"
)

// Format controls the log output format.
type Format int

const (
	// FormatJSON emits newline-delimited JSON records.
	FormatJSON Format = iota
	// FormatText emits human-readable key=value records.
	FormatText
)

// LoggerOpts configures the logger.
type LoggerOpts struct {
	// Format selects JSON or text output. Default: FormatJSON.
	Format Format
	// Level sets the minimum log level. Default: slog.LevelInfo.
	Level slog.Level
	// Output is the writer for log records. Default: os.Stderr.
	Output io.Writer
}

// Logger wraps slog.Logger with C4-specific defaults.
// Use SetLevel to adjust the minimum log level at runtime.
type Logger struct {
	slog     *slog.Logger
	levelVar *slog.LevelVar
}

// NewLogger creates a Logger with the given options.
// Zero-value LoggerOpts produce a JSON logger writing to os.Stderr at Info level.
func NewLogger(opts LoggerOpts) *Logger {
	out := opts.Output
	if out == nil {
		out = os.Stderr
	}

	lv := &slog.LevelVar{}
	lv.Set(opts.Level)
	ho := &slog.HandlerOptions{Level: lv}

	var h slog.Handler
	if opts.Format == FormatText {
		h = slog.NewTextHandler(out, ho)
	} else {
		h = slog.NewJSONHandler(out, ho)
	}

	return &Logger{slog: slog.New(h), levelVar: lv}
}

// SetLevel updates the minimum log level at runtime without recreating the logger.
func (l *Logger) SetLevel(level slog.Level) {
	l.levelVar.Set(level)
}

// Level returns the current minimum log level.
func (l *Logger) Level() slog.Level {
	return l.levelVar.Level()
}

// Info logs a message at Info level with optional key-value pairs.
func (l *Logger) Info(msg string, args ...any) {
	l.slog.Info(msg, args...)
}

// Debug logs a message at Debug level with optional key-value pairs.
func (l *Logger) Debug(msg string, args ...any) {
	l.slog.Debug(msg, args...)
}

// Warn logs a message at Warn level with optional key-value pairs.
func (l *Logger) Warn(msg string, args ...any) {
	l.slog.Warn(msg, args...)
}

// Error logs a message at Error level with optional key-value pairs.
func (l *Logger) Error(msg string, args ...any) {
	l.slog.Error(msg, args...)
}

// Slog returns the underlying slog.Logger for interoperability.
func (l *Logger) Slog() *slog.Logger {
	return l.slog
}
