package main

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/AgSpades/tele-trader.git/internal/llm"
	"github.com/AgSpades/tele-trader.git/internal/models"
)

const (
	signalBatchWindow = 900 * time.Millisecond
	signalBatchMax    = 12
	signalWorkers     = 3
	signalTimeout     = 5 * time.Minute
)

type signalProcessor struct {
	llmClient *llm.Client
	jobs      chan string
	wg        sync.WaitGroup
}

func newSignalProcessor(llmClient *llm.Client) *signalProcessor {
	return &signalProcessor{
		llmClient: llmClient,
		jobs:      make(chan string, 64),
	}
}

func (p *signalProcessor) Start(ctx context.Context) {
	for i := 0; i < signalWorkers; i++ {
		workerID := i + 1
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.worker(ctx, workerID)
		}()
	}
}

func (p *signalProcessor) Stop() {
	p.wg.Wait()
}

func (p *signalProcessor) Enqueue(ctx context.Context, signal string) {
	select {
	case p.jobs <- signal:
	case <-ctx.Done():
	}
}

func (p *signalProcessor) worker(ctx context.Context, workerID int) {
	slog.Info("tele-trader: signal worker armed", "worker_id", workerID)
	for {
		select {
		case <-ctx.Done():
			slog.Info("tele-trader: signal worker shutting down", "worker_id", workerID)
			return
		case msg, ok := <-p.jobs:
			if !ok {
				slog.Info("tele-trader: signal worker stopped", "worker_id", workerID)
				return
			}
			processSignal(ctx, p.llmClient, msg, workerID)
		}
	}
}

func runSignalBatcher(ctx context.Context, in <-chan string, processor *signalProcessor) {
	var batch []string
	var timer *time.Timer
	var timerC <-chan time.Time

	flush := func(reason string) {
		if len(batch) == 0 {
			return
		}
		merged := strings.Join(batch, "\n")
		slog.Info("tele-trader: signal batch ready", "count", len(batch), "reason", reason, "signal", merged)
		processor.Enqueue(ctx, merged)
		batch = nil
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer = nil
			timerC = nil
		}
	}

	for {
		select {
		case <-ctx.Done():
			flush("shutdown")
			return
		case msg, ok := <-in:
			if !ok {
				flush("input_closed")
				return
			}
			batch = append(batch, msg)
			if len(batch) == 1 {
				timer = time.NewTimer(signalBatchWindow)
				timerC = timer.C
			}
			if len(batch) >= signalBatchMax {
				flush("max_size")
			}
		case <-timerC:
			flush("window_elapsed")
		}
	}
}

// processSignal wraps the LLM signal processing with timeout and logging.
func processSignal(ctx context.Context, llmClient *llm.Client, msg string, workerID int) {
	sigCtx, cancel := context.WithTimeout(ctx, signalTimeout)
	defer cancel()

	signal := models.Signal{
		RawText:    msg,
		CleanText:  msg,
		ReceivedAt: time.Now(),
	}

	slog.Info("tele-trader: processing signal", "worker_id", workerID, "signal", msg)

	result, err := llmClient.ProcessSignal(sigCtx, signal)
	if err != nil {
		slog.Error("tele-trader: llm processing error", "worker_id", workerID, "error", err, "signal", msg)
		return
	}

	slog.Info("tele-trader: agent response", "worker_id", workerID, "response", result)
}
