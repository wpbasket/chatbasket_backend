package logger

import (
	"log/slog"
	"os"
)

// New creates a new structured logger using slog with JSON handler, exactly as in chatbasket-api/app/main.go
func New() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil))
}
