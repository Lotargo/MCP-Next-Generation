package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

var logLevel = new(slog.LevelVar)

// SetLevel sets the global logging level dynamically.
func SetLevel(level slog.Level) {
	logLevel.Set(level)
}

// SetupLogger configures the default slog logger to write JSON logs to the specified file
// and to stderr.
func SetupLogger(logFilePath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(logFilePath), 0755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	// Default to INFO
	logLevel.Set(slog.LevelInfo)

	// Write to both file and stderr
	w := io.MultiWriter(f, os.Stderr)

	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: logLevel,
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return func() {
		f.Close()
	}, nil
}
