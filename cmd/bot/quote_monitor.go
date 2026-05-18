package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/AgSpades/tele-trader.git/internal/broker"
	"github.com/AgSpades/tele-trader.git/internal/llm"
	"github.com/AgSpades/tele-trader.git/internal/models"
)

const quoteMonitorInterval = 2 * time.Second

type quoteMonitor struct {
	brokerClient *broker.Client
	llmClient    *llm.Client
	processor    *signalProcessor
	lastEvents   map[string]string
}

func newQuoteMonitor(
	brokerClient *broker.Client,
	llmClient *llm.Client,
	processor *signalProcessor,
) *quoteMonitor {
	return &quoteMonitor{
		brokerClient: brokerClient,
		llmClient:    llmClient,
		processor:    processor,
		lastEvents:   make(map[string]string),
	}
}

func (m *quoteMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(quoteMonitorInterval)
	defer ticker.Stop()

	slog.Info("quote-monitor: armed", "interval", quoteMonitorInterval)
	for {
		select {
		case <-ctx.Done():
			slog.Info("quote-monitor: shutting down")
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

func (m *quoteMonitor) check(ctx context.Context) {
	for _, trade := range m.llmClient.SnapshotTrades() {
		if trade.Status != "pending" && trade.Status != "active" {
			continue
		}
		if trade.Symbol == "" || trade.Exchange == "" {
			continue
		}

		raw, err := m.brokerClient.GetQuote(ctx, models.GetQuoteParams{
			Symbol:   trade.Symbol,
			Exchange: trade.Exchange,
		})
		if err != nil {
			slog.Warn("quote-monitor: quote failed", "symbol", trade.Symbol, "error", err)
			continue
		}

		ltp, ok := extractQuoteLTP(raw)
		if !ok {
			slog.Warn("quote-monitor: quote missing ltp", "symbol", trade.Symbol, "response", string(raw))
			continue
		}

		if event := m.eventForTrade(trade, ltp); event != "" {
			key := trade.ID
			if key == "" {
				key = trade.Symbol
			}
			eventKey := fmt.Sprintf("%s:%.2f", event, ltp)
			if m.lastEvents[key] == eventKey {
				continue
			}
			m.lastEvents[key] = eventKey

			msg := formatQuoteMonitorEvent(trade, event, ltp)
			slog.Info("quote-monitor: emitting event", "symbol", trade.Symbol, "event", event, "ltp", ltp)
			m.processor.Enqueue(ctx, msg)
		}
	}
}

func (m *quoteMonitor) eventForTrade(trade llm.TradeMemory, ltp float64) string {
	switch trade.Status {
	case "pending":
		if trade.EntryTrigger == 0 {
			return ""
		}
		upper := trade.EntryUpper
		if upper == 0 {
			upper = trade.EntryTrigger
		}
		maxEntry := upper * 1.05
		if ltp >= trade.EntryTrigger && ltp <= maxEntry {
			return "PENDING_TRIGGER_HIT"
		}
	case "active":
		if trade.TargetsHit >= len(trade.Targets) {
			return ""
		}
		nextTarget := trade.Targets[trade.TargetsHit]
		if nextTarget != 0 && ltp >= nextTarget {
			return "TARGET_HIT"
		}
	}
	return ""
}

func formatQuoteMonitorEvent(trade llm.TradeMemory, event string, ltp float64) string {
	return fmt.Sprintf(`LIVE_QUOTE_UPDATE
event: %s
id: %s
status: %s
symbol: %s
exchange: %s
index: %s
strike: %d
option_type: %s
ltp: %.2f
entry_trigger: %.2f
entry_upper: %.2f
entry_price: %.2f
stop_loss: %.2f
targets: %s
targets_hit: %d
quantity: %d
sl_order_id: %s
instruction: Use TRADING_MEMORY and this live quote to enter, trail, exit, or ignore according to the system rules.`,
		event,
		trade.ID,
		trade.Status,
		trade.Symbol,
		trade.Exchange,
		trade.Index,
		trade.Strike,
		trade.OptionType,
		ltp,
		trade.EntryTrigger,
		trade.EntryUpper,
		trade.EntryPrice,
		trade.StopLoss,
		formatFloatSlice(trade.Targets),
		trade.TargetsHit,
		trade.Quantity,
		trade.SLOrderID,
	)
}

func extractQuoteLTP(raw json.RawMessage) (float64, bool) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, false
	}
	return findLTP(v)
}

func findLTP(v interface{}) (float64, bool) {
	switch typed := v.(type) {
	case map[string]interface{}:
		for key, value := range typed {
			if strings.EqualFold(key, "ltp") {
				return numberValue(value)
			}
		}
		for _, value := range typed {
			if ltp, ok := findLTP(value); ok {
				return ltp, true
			}
		}
	case []interface{}:
		for _, value := range typed {
			if ltp, ok := findLTP(value); ok {
				return ltp, true
			}
		}
	}
	return 0, false
}

func numberValue(v interface{}) (float64, bool) {
	switch typed := v.(type) {
	case float64:
		return typed, !math.IsNaN(typed)
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func formatFloatSlice(values []float64) string {
	if len(values) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(value, 'f', 2, 64))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
