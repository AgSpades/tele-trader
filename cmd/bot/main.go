// cmd/bot/main.go is the application entry point for the tele-trader bot.
// It wires all packages together, starts the Telegram userbot, and runs the
// signal-processing loop with graceful shutdown support.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AgSpades/tele-trader.git/internal/broker"
	"github.com/AgSpades/tele-trader.git/internal/config"
	"github.com/AgSpades/tele-trader.git/internal/llm"
	telegramPkg "github.com/AgSpades/tele-trader.git/internal/telegram"
)

func main() {
	// --- Structured JSON logging ---
	logFile, logPath, err := setupLogger(time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "tele-trader: failed to set up logging: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	slog.Info("tele-trader: starting up", "log_file", logPath)

	// --- Configuration ---
	cfg, err := config.Load()
	if err != nil {
		slog.Error("tele-trader: failed to load config", "error", err)
		os.Exit(1)
	}

	if cfg.DryRun {
		slog.Warn("tele-trader: DRY_RUN=true — orders will be routed through OpenAlgo Analyzer (no real trades)")
	}

	// --- Context with graceful shutdown ---
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// --- Dependencies ---
	brokerClient := broker.New(cfg.OpenAlgoAPIKey, cfg.OpenAlgoURL)
	llmClient := llm.New(cfg, brokerClient)
	session := telegramPkg.NewFileSessionStorage(cfg.SessionFilePath)
	tgHandler := telegramPkg.NewHandler(
		cfg.TelegramAppID,
		cfg.TelegramAppHash,
		cfg.TelegramPhone,
		cfg.TelegramChannelID,
		session,
	)

	// --- Signal channel (buffered to absorb bursts) ---
	msgCh := make(chan string, 32)

	// --- Startup Health Checks ---
	errCh := make(chan error, 1)
	go func() {
		if err := tgHandler.Start(ctx, msgCh); err != nil && ctx.Err() == nil {
			errCh <- fmt.Errorf("telegram handler error: %w", err)
		}
	}()

	isFreshLogin := false
	if _, err := os.Stat(cfg.SessionFilePath); os.IsNotExist(err) {
		isFreshLogin = true
	}

	slog.Info("tele-trader: performing startup health checks")
	if err := runStartupChecks(ctx, brokerClient, tgHandler, errCh, isFreshLogin); err != nil {
		slog.Error("tele-trader: startup health check failed", "error", err)
		os.Exit(1)
	}

	// --- Signal processor workers and burst batcher ---
	processor := newSignalProcessor(llmClient)
	processor.Start(ctx)
	defer processor.Stop()

	go runSignalBatcher(ctx, msgCh, processor)
	go newQuoteMonitor(brokerClient, llmClient, processor).Run(ctx)

	go func() {
		select {
		case err := <-errCh:
			slog.Error("tele-trader: fatal background error", "error", err)
			os.Exit(1)
		case <-ctx.Done():
			return
		}
	}()

	<-ctx.Done()
	slog.Info("tele-trader: shutdown complete")
}

// runStartupChecks verifies OpenAlgo API connectivity and waits for the
// Telegram session to be authenticated before the signal processor is armed.
// Both checks run concurrently. Returns an error if either check fails within
// the configured timeout. If isFreshLogin is true, the timeout is disabled
// to allow the user unlimited time to complete the interactive auth flow.
func runStartupChecks(
	ctx context.Context,
	brokerClient *broker.Client,
	tgHandler *telegramPkg.Handler,
	tgErrCh <-chan error,
	isFreshLogin bool,
) error {
	var checkCtx context.Context
	var cancel context.CancelFunc

	if isFreshLogin {
		slog.Info("healthcheck: fresh login detected, waiting indefinitely for Telegram auth…")
		checkCtx, cancel = context.WithCancel(ctx)
	} else {
		timeout := startupCheckTimeout()
		slog.Info("healthcheck: starting", "timeout", timeout)
		checkCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	type result struct {
		name string
		err  error
	}
	results := make(chan result, 2)

	// OpenAlgo check — verify API key and server reachability.
	go func() {
		slog.Info("healthcheck: verifying OpenAlgo connectivity…")
		if err := brokerClient.Ping(checkCtx); err != nil {
			results <- result{"OpenAlgo", err}
			return
		}
		slog.Info("healthcheck: ✓ OpenAlgo connected")
		results <- result{"OpenAlgo", nil}
	}()

	// Telegram check — wait for the Handler to close ReadyCh (auth complete).
	go func() {
		slog.Info("healthcheck: waiting for Telegram authentication…")
		select {
		case <-tgHandler.ReadyCh():
			slog.Info("healthcheck: ✓ Telegram authenticated")
			results <- result{"Telegram", nil}
		case err := <-tgErrCh:
			// Telegram goroutine exited before becoming ready.
			if err == nil {
				err = fmt.Errorf("telegram handler exited unexpectedly")
			}
			results <- result{"Telegram", err}
		case <-checkCtx.Done():
			results <- result{"Telegram", fmt.Errorf("timed out: %w", checkCtx.Err())}
		}
	}()

	// Collect results from both checks.
	var failed int
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			slog.Error("healthcheck: FAILED", "service", r.name, "error", r.err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d startup health check(s) failed — see logs above", failed)
	}
	slog.Info("healthcheck: all checks passed — signal processor arming")
	return nil
}

// startupCheckTimeout returns the health-check deadline.
// Defaults to 120 s to allow time for interactive OTP/2FA entry.
// Override with STARTUP_CHECK_TIMEOUT_SEC (value in seconds).
func startupCheckTimeout() time.Duration {
	const def = 120 * time.Second
	v := os.Getenv("STARTUP_CHECK_TIMEOUT_SEC")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v + "s")
	if err != nil {
		slog.Warn("healthcheck: invalid STARTUP_CHECK_TIMEOUT_SEC, using default", "default", def)
		return def
	}
	return d
}
