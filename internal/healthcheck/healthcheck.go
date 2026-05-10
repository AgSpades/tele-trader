// Package healthcheck provides a startup gate that validates external
// service connectivity before the signal processor is armed.
//
// Run() performs two checks in parallel:
//  1. OpenAlgo — calls the Funds API and asserts the API key is accepted.
//  2. Telegram  — waits for the Handler to close its ReadyCh sentinel,
//     confirming that authentication (OTP / 2FA) has completed and
//     client.Self() has returned successfully.
//
// If either check fails or the overall timeout is exceeded, Run returns a
// descriptive error; the caller should log it and exit.
package healthcheck

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Pinger is satisfied by broker.Client.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Run performs the startup health checks with the given timeout.
// It blocks until both services report healthy or the timeout elapses.
// Returns nil only when all checks pass.
func Run(ctx context.Context, broker Pinger, tgReadyCh <-chan struct{}, timeout time.Duration) error {
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		name string
		err  error
	}
	results := make(chan result, 2)

	// --- OpenAlgo check ---
	go func() {
		slog.Info("healthcheck: verifying OpenAlgo connectivity…")
		if err := broker.Ping(checkCtx); err != nil {
			results <- result{"OpenAlgo", err}
			return
		}
		slog.Info("healthcheck: ✓ OpenAlgo connected")
		results <- result{"OpenAlgo", nil}
	}()

	// --- Telegram check ---
	go func() {
		slog.Info("healthcheck: waiting for Telegram authentication…")
		select {
		case <-tgReadyCh:
			slog.Info("healthcheck: ✓ Telegram authenticated")
			results <- result{"Telegram", nil}
		case <-checkCtx.Done():
			results <- result{"Telegram", fmt.Errorf("timed out waiting for Telegram authentication: %w", checkCtx.Err())}
		}
	}()

	// Collect both results.
	var errs []error
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", r.name, r.err))
		}
	}

	if len(errs) > 0 {
		for _, e := range errs {
			slog.Error("healthcheck: FAILED", "error", e)
		}
		return fmt.Errorf("startup health checks failed (%d/%d checks passed)", 2-len(errs), 2)
	}

	slog.Info("healthcheck: all checks passed — signal processor arming")
	return nil
}
