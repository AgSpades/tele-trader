package main

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSetupLoggerWritesDatedFile(t *testing.T) {
	tmp := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	closer, logPath, err := setupLogger(time.Date(2026, 5, 18, 9, 30, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("setupLogger: %v", err)
	}

	slog.Info("test log entry", "component", "logger")
	if err := closer.Close(); err != nil {
		t.Fatalf("close log file: %v", err)
	}

	wantPath := filepath.Join(logsDir, "18052026.log")
	if logPath != wantPath {
		t.Fatalf("logPath = %q, want %q", logPath, wantPath)
	}

	file, err := os.Open(wantPath)
	if err != nil {
		t.Fatalf("open log file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatalf("expected at least one log line")
	}
	line := scanner.Text()
	if !strings.Contains(line, `"msg":"test log entry"`) {
		t.Fatalf("log line missing message: %s", line)
	}
	if !strings.Contains(line, `"component":"logger"`) {
		t.Fatalf("log line missing attr: %s", line)
	}
}
