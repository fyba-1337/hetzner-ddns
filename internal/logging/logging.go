package logging

import (
	"log/slog"
	"os"
	"strings"
)

func New(level slog.Level, format string) *slog.Logger {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "json":
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		}))
	default:
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		}))
	}
}
