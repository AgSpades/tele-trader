package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	tradeStatusPending = "pending"
	tradeStatusActive  = "active"
	tradeStatusClosed  = "closed"
	maxTradeMemory     = 32
)

// TradeMemory is the durable, structured context the agent needs across
// Telegram messages that omit the option symbol.
type TradeMemory struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	Index        string    `json:"index,omitempty"`
	Strike       int       `json:"strike,omitempty"`
	OptionType   string    `json:"option_type,omitempty"`
	Exchange     string    `json:"exchange,omitempty"`
	Symbol       string    `json:"symbol,omitempty"`
	EntryTrigger float64   `json:"entry_trigger,omitempty"`
	EntryUpper   float64   `json:"entry_upper,omitempty"`
	EntryPrice   float64   `json:"entry_price,omitempty"`
	StopLoss     float64   `json:"stop_loss,omitempty"`
	Targets      []float64 `json:"targets,omitempty"`
	TargetsHit   int       `json:"targets_hit,omitempty"`
	Quantity     int       `json:"quantity,omitempty"`
	SLOrderID    string    `json:"sl_order_id,omitempty"`
	LastMessage  string    `json:"last_message,omitempty"`
	Notes        string    `json:"notes,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type upsertTradeMemoryParams struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	Index        string    `json:"index"`
	Strike       int       `json:"strike"`
	OptionType   string    `json:"option_type"`
	Exchange     string    `json:"exchange"`
	Symbol       string    `json:"symbol"`
	EntryTrigger float64   `json:"entry_trigger"`
	EntryUpper   float64   `json:"entry_upper"`
	EntryPrice   float64   `json:"entry_price"`
	StopLoss     float64   `json:"stop_loss"`
	Targets      []float64 `json:"targets"`
	TargetsHit   int       `json:"targets_hit"`
	Quantity     int       `json:"quantity"`
	SLOrderID    string    `json:"sl_order_id"`
	LastMessage  string    `json:"last_message"`
	Notes        string    `json:"notes"`
}

type closeTradeMemoryParams struct {
	ID     string `json:"id"`
	Symbol string `json:"symbol"`
	Reason string `json:"reason"`
}

func (c *Client) memorySnapshot() string {
	if len(c.tradeMemory) == 0 {
		return "[]"
	}
	b, err := json.MarshalIndent(c.tradeMemory, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(b)
}

func (c *Client) upsertTradeMemory(p upsertTradeMemoryParams) json.RawMessage {
	now := time.Now().UTC()
	next := TradeMemory{
		ID:           strings.TrimSpace(p.ID),
		Status:       normalizeStatus(p.Status),
		Index:        strings.ToUpper(strings.TrimSpace(p.Index)),
		Strike:       p.Strike,
		OptionType:   strings.ToUpper(strings.TrimSpace(p.OptionType)),
		Exchange:     strings.ToUpper(strings.TrimSpace(p.Exchange)),
		Symbol:       strings.TrimSpace(p.Symbol),
		EntryTrigger: p.EntryTrigger,
		EntryUpper:   p.EntryUpper,
		EntryPrice:   p.EntryPrice,
		StopLoss:     p.StopLoss,
		Targets:      p.Targets,
		TargetsHit:   p.TargetsHit,
		Quantity:     p.Quantity,
		SLOrderID:    strings.TrimSpace(p.SLOrderID),
		LastMessage:  strings.TrimSpace(p.LastMessage),
		Notes:        strings.TrimSpace(p.Notes),
		UpdatedAt:    now,
	}
	if next.ID == "" {
		next.ID = deriveTradeMemoryID(next)
	}

	for i := range c.tradeMemory {
		if sameTradeMemory(c.tradeMemory[i], next) {
			mergeTradeMemory(&c.tradeMemory[i], next)
			return c.memoryToolResult("updated", c.tradeMemory[i])
		}
	}

	c.tradeMemory = append(c.tradeMemory, next)
	if len(c.tradeMemory) > maxTradeMemory {
		c.tradeMemory = c.tradeMemory[len(c.tradeMemory)-maxTradeMemory:]
	}
	return c.memoryToolResult("created", next)
}

func (c *Client) closeTradeMemory(p closeTradeMemoryParams) json.RawMessage {
	id := strings.TrimSpace(p.ID)
	symbol := strings.TrimSpace(p.Symbol)
	reason := strings.TrimSpace(p.Reason)

	for i := range c.tradeMemory {
		if (id != "" && c.tradeMemory[i].ID == id) || (symbol != "" && c.tradeMemory[i].Symbol == symbol) {
			c.tradeMemory[i].Status = tradeStatusClosed
			c.tradeMemory[i].Notes = mergeNotes(c.tradeMemory[i].Notes, reason)
			c.tradeMemory[i].UpdatedAt = time.Now().UTC()
			return c.memoryToolResult("closed", c.tradeMemory[i])
		}
	}

	b, _ := json.Marshal(map[string]string{
		"status":  "not_found",
		"message": "no matching trade memory found",
	})
	return b
}

func (c *Client) memoryToolResult(status string, trade TradeMemory) json.RawMessage {
	b, _ := json.Marshal(map[string]interface{}{
		"status": status,
		"trade":  trade,
		"memory": c.tradeMemory,
	})
	return b
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case tradeStatusPending, tradeStatusActive, tradeStatusClosed:
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return tradeStatusPending
	}
}

func deriveTradeMemoryID(t TradeMemory) string {
	if t.Symbol != "" {
		return t.Symbol
	}
	if t.Index != "" && t.Strike != 0 && t.OptionType != "" {
		return fmt.Sprintf("%s-%d-%s", t.Index, t.Strike, t.OptionType)
	}
	return fmt.Sprintf("trade-%d", time.Now().UnixNano())
}

func sameTradeMemory(a, b TradeMemory) bool {
	if a.ID != "" && b.ID != "" && a.ID == b.ID {
		return true
	}
	if a.Symbol != "" && b.Symbol != "" && a.Symbol == b.Symbol {
		return true
	}
	return a.Index != "" &&
		a.Index == b.Index &&
		a.Strike != 0 &&
		a.Strike == b.Strike &&
		a.OptionType != "" &&
		a.OptionType == b.OptionType
}

func mergeTradeMemory(dst *TradeMemory, src TradeMemory) {
	dst.Status = src.Status
	if src.Index != "" {
		dst.Index = src.Index
	}
	if src.Strike != 0 {
		dst.Strike = src.Strike
	}
	if src.OptionType != "" {
		dst.OptionType = src.OptionType
	}
	if src.Exchange != "" {
		dst.Exchange = src.Exchange
	}
	if src.Symbol != "" {
		dst.Symbol = src.Symbol
	}
	if src.EntryTrigger != 0 {
		dst.EntryTrigger = src.EntryTrigger
	}
	if src.EntryUpper != 0 {
		dst.EntryUpper = src.EntryUpper
	}
	if src.EntryPrice != 0 {
		dst.EntryPrice = src.EntryPrice
	}
	if src.StopLoss != 0 {
		dst.StopLoss = src.StopLoss
	}
	if src.Targets != nil {
		dst.Targets = src.Targets
	}
	if src.TargetsHit != 0 {
		dst.TargetsHit = src.TargetsHit
	}
	if src.Quantity != 0 {
		dst.Quantity = src.Quantity
	}
	if src.SLOrderID != "" {
		dst.SLOrderID = src.SLOrderID
	}
	if src.LastMessage != "" {
		dst.LastMessage = src.LastMessage
	}
	if src.Notes != "" {
		dst.Notes = mergeNotes(dst.Notes, src.Notes)
	}
	dst.UpdatedAt = src.UpdatedAt
}

func mergeNotes(existing, next string) string {
	switch {
	case existing == "":
		return next
	case next == "":
		return existing
	case strings.Contains(existing, next):
		return existing
	default:
		return existing + "; " + next
	}
}

// SnapshotTrades returns a copy of current structured trade memory.
func (c *Client) SnapshotTrades() []TradeMemory {
	c.mu.Lock()
	defer c.mu.Unlock()

	trades := make([]TradeMemory, len(c.tradeMemory))
	copy(trades, c.tradeMemory)
	for i := range trades {
		if trades[i].Targets != nil {
			trades[i].Targets = append([]float64(nil), trades[i].Targets...)
		}
	}
	return trades
}
