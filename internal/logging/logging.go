// Package logging constructs the process-wide structured logger. Log calls
// must use allowlisted fields and must never include authorization material.
package logging

import (
	"fmt"
	"io"
	"log/slog"
)

// New returns a JSON logger at the configured level.
func New(output io.Writer, level string) (*slog.Logger, error) {
	if output == nil {
		return nil, fmt.Errorf("log output is required")
	}

	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		return nil, fmt.Errorf("unsupported log level")
	}

	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: slogLevel})), nil
}
