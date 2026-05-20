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

		switch trade.Status {
		case "pending":
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
		case "active":
			m.handleActiveTrade(ctx, trade, ltp)
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

func (m *quoteMonitor) handleActiveTrade(ctx context.Context, trade llm.TradeMemory, ltp float64) {
	if trade.TargetsHit >= len(trade.Targets) || len(trade.Targets) == 0 {
		m.checkStopLossFilled(ctx, trade, ltp)
		return
	}
	if trade.Quantity <= 0 {
		slog.Warn("quote-monitor: missing quantity for trailing", "symbol", trade.Symbol)
		m.checkStopLossFilled(ctx, trade, ltp)
		return
	}

	newTargetsHit := targetsHitForLTP(trade.Targets, ltp)
	if newTargetsHit <= trade.TargetsHit {
		m.checkStopLossFilled(ctx, trade, ltp)
		return
	}

	newSL := trailingStopLoss(trade, newTargetsHit)
	if newSL <= trade.StopLoss {
		m.checkStopLossFilled(ctx, trade, ltp)
		return
	}

	slOrderID := strings.TrimSpace(trade.SLOrderID)
	if slOrderID == "" {
		order, ok := m.findProtectiveOrder(ctx, trade.Symbol)
		if ok {
			slOrderID = order.OrderID
		}
	}

	if slOrderID == "" {
		slog.Warn("quote-monitor: no SL order id to trail", "symbol", trade.Symbol)
		return
	}

	price := newSL - 2
	if price <= 0 {
		price = newSL
	}

	modify := models.ModifyOrderParams{
		OrderID:      slOrderID,
		Symbol:       trade.Symbol,
		Action:       "SELL",
		Exchange:     trade.Exchange,
		PriceType:    "SL",
		Product:      broker.DefaultProduct,
		Quantity:     trade.Quantity,
		TriggerPrice: newSL,
		Price:        price,
	}

	if _, err := m.brokerClient.ModifyOrder(ctx, modify); err != nil {
		slog.Warn("quote-monitor: failed to trail SL", "symbol", trade.Symbol, "order_id", slOrderID, "error", err)
		return
	}

	updated := trade
	updated.TargetsHit = newTargetsHit
	updated.StopLoss = newSL
	updated.SLOrderID = slOrderID
	updated.Notes = fmt.Sprintf("trailed SL to %.2f after target %d", newSL, newTargetsHit)
	m.llmClient.UpdateTradeMemorySystem(updated)

	slog.Info("quote-monitor: trailed SL", "symbol", trade.Symbol, "new_sl", newSL, "targets_hit", newTargetsHit)
}

func (m *quoteMonitor) checkStopLossFilled(ctx context.Context, trade llm.TradeMemory, ltp float64) {
	if trade.StopLoss == 0 {
		return
	}
	if ltp > trade.StopLoss {
		return
	}

	if trade.SLOrderID == "" {
		return
	}

	order, ok := m.getOrderByID(ctx, trade.SLOrderID)
	if !ok {
		return
	}
	if !order.IsComplete {
		return
	}

	if _, closed := m.llmClient.CloseTradeMemorySystem(trade.ID, trade.Symbol, "stop loss filled"); closed {
		slog.Info("quote-monitor: SL filled, trade closed", "symbol", trade.Symbol, "order_id", trade.SLOrderID)
	}
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

type orderBookEntry struct {
	OrderID    string
	Symbol     string
	Action     string
	PriceType  string
	OrderState string
	IsComplete bool
}

func (m *quoteMonitor) findProtectiveOrder(ctx context.Context, symbol string) (orderBookEntry, bool) {
	orderBook, err := m.brokerClient.GetOrderBook(ctx)
	if err != nil {
		slog.Warn("quote-monitor: order book failed", "error", err)
		return orderBookEntry{}, false
	}

	orders, ok := extractOrders(orderBook)
	if !ok {
		return orderBookEntry{}, false
	}

	for _, order := range orders {
		if !strings.EqualFold(order.Symbol, symbol) {
			continue
		}
		if !strings.EqualFold(order.Action, "SELL") {
			continue
		}
		if !strings.EqualFold(order.PriceType, "SL") {
			continue
		}
		if order.IsComplete {
			continue
		}
		return order, true
	}

	return orderBookEntry{}, false
}

func (m *quoteMonitor) getOrderByID(ctx context.Context, orderID string) (orderBookEntry, bool) {
	orderBook, err := m.brokerClient.GetOrderBook(ctx)
	if err != nil {
		slog.Warn("quote-monitor: order book failed", "error", err)
		return orderBookEntry{}, false
	}

	orders, ok := extractOrders(orderBook)
	if !ok {
		return orderBookEntry{}, false
	}

	for _, order := range orders {
		if order.OrderID == orderID {
			return order, true
		}
	}

	return orderBookEntry{}, false
}

func extractOrders(raw json.RawMessage) ([]orderBookEntry, bool) {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}

	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		return nil, false
	}

	ordersRaw, ok := data["orders"].([]interface{})
	if !ok {
		return nil, false
	}

	orders := make([]orderBookEntry, 0, len(ordersRaw))
	for _, item := range ordersRaw {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		order := orderBookEntry{
			OrderID:    stringValue(entry["orderid"]),
			Symbol:     stringValue(entry["symbol"]),
			Action:     stringValue(entry["action"]),
			PriceType:  stringValue(entry["pricetype"]),
			OrderState: stringValue(entry["order_status"]),
		}
		order.IsComplete = strings.EqualFold(order.OrderState, "complete")
		orders = append(orders, order)
	}

	return orders, true
}

func stringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch typed := v.(type) {
	case string:
		return typed
	case float64:
		if typed == 0 {
			return "0"
		}
		return strconv.FormatFloat(typed, 'f', 0, 64)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func targetsHitForLTP(targets []float64, ltp float64) int {
	count := 0
	for _, target := range targets {
		if target != 0 && ltp >= target {
			count++
		}
	}
	return count
}

func trailingStopLoss(trade llm.TradeMemory, targetsHit int) float64 {
	if targetsHit <= 0 {
		return trade.StopLoss
	}
	if targetsHit == 1 {
		if trade.EntryPrice != 0 {
			return trade.EntryPrice
		}
		if trade.EntryTrigger != 0 {
			return trade.EntryTrigger
		}
		return trade.StopLoss
	}
	idx := targetsHit - 2
	if idx >= 0 && idx < len(trade.Targets) {
		return trade.Targets[idx]
	}
	return trade.StopLoss
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
