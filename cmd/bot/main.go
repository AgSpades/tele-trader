// cmd/bot/main.go is the application entry point for the tele-trader bot.
// It wires all packages together, starts the Telegram userbot, and runs the
// signal-processing loop with graceful shutdown support.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AgSpades/tele-trader.git/internal/broker"
	"github.com/AgSpades/tele-trader.git/internal/config"
	"github.com/AgSpades/tele-trader.git/internal/llm"
	"github.com/AgSpades/tele-trader.git/internal/models"
	telegramPkg "github.com/AgSpades/tele-trader.git/internal/telegram"
)

func main() {
	// --- Structured JSON logging ---
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("tele-trader: starting up")

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
	llmClient := llm.New(cfg.AnthropicAPIKey, brokerClient)
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

	// --- Signal processor goroutine ---
	go func() {
		slog.Info("tele-trader: signal processor started")
		for {
			select {
			case <-ctx.Done():
				slog.Info("tele-trader: signal processor shutting down")
				return
			case rawMsg, ok := <-msgCh:
				if !ok {
					return
				}
				processSignal(ctx, llmClient, rawMsg)
			}
		}
	}()

	// --- Telegram listener (blocks until ctx cancelled) ---
	slog.Info("tele-trader: starting telegram listener")
	if err := tgHandler.Start(ctx, msgCh); err != nil && ctx.Err() == nil {
		// Only log as error if we weren't shut down intentionally.
		slog.Error("tele-trader: telegram handler exited with error", "error", err)
		os.Exit(1)
	}

	slog.Info("tele-trader: shutdown complete")
}

// processSignal wraps the LLM signal processing with timeout and logging.
func processSignal(ctx context.Context, llmClient *llm.Client, msg string) {
	// Per-signal timeout to prevent a single stalled API call from blocking the processor.
	sigCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	signal := models.Signal{
		RawText:    msg,
		CleanText:  msg, // already cleaned by telegram handler
		ReceivedAt: time.Now(),
	}

	slog.Info("tele-trader: processing signal", "signal", msg)

	result, err := llmClient.ProcessSignal(sigCtx, signal)
	if err != nil {
		slog.Error("tele-trader: llm processing error", "error", err, "signal", msg)
		return
	}

	slog.Info("tele-trader: agent response", "response", result)
}
