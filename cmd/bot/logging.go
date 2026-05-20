package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const logsDir = "logs"

type multiHandler struct {
	handlers []slog.Handler
}

func (h multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var firstErr error
	for _, handler := range h.handlers {
		if err := handler.Handle(ctx, record.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (h multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}
	return multiHandler{handlers: handlers}
}

func (h multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}
	return multiHandler{handlers: handlers}
}

func setupLogger(now time.Time) (io.Closer, string, error) {
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, "", fmt.Errorf("create logs directory: %w", err)
	}

	logPath := filepath.Join(logsDir, now.Format("02012006")+".log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, "", fmt.Errorf("open log file: %w", err)
	}

	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	logger := slog.New(multiHandler{handlers: []slog.Handler{
		slog.NewJSONHandler(os.Stdout, opts),
		slog.NewJSONHandler(file, opts),
	}})
	slog.SetDefault(logger)

	return file, logPath, nil
}
