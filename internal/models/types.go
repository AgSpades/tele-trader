// Package models defines shared domain types used across the application.
package models

import (
	"encoding/json"
	"time"
)

// Signal represents a cleaned message received from Telegram.
type Signal struct {
	RawText    string
	CleanText  string
	ReceivedAt time.Time
}

// ToolCall represents a single tool invocation requested by the LLM.
type ToolCall struct {
	ID    string          // tool_use block id from Claude
	Name  string          // tool name, e.g. "place_order"
	Input json.RawMessage // raw JSON input parameters
}

// ToolResult carries the JSON response of a broker call back to the LLM.
type ToolResult struct {
	ToolUseID string          // must match ToolCall.ID
	Content   json.RawMessage // broker response JSON
	IsError   bool
}

// PlaceOrderParams holds the parameters for placing an order via the broker.
type PlaceOrderParams struct {
	Strategy     string  `json:"strategy"`
	Symbol       string  `json:"symbol"`
	Action       string  `json:"action"`       // BUY or SELL
	Exchange     string  `json:"exchange"`     // NFO, BFO, NSE, etc.
	PriceType    string  `json:"price_type"`   // MARKET, LIMIT, SL
	Product      string  `json:"product"`      // MIS (default), NRML
	Quantity     int     `json:"quantity"`
	Price        float64 `json:"price,omitempty"`
	TriggerPrice float64 `json:"trigger_price,omitempty"`
}

// ModifyOrderParams holds the parameters for modifying an existing SL order.
type ModifyOrderParams struct {
	OrderID      string  `json:"order_id"`
	Strategy     string  `json:"strategy"`
	Symbol       string  `json:"symbol"`
	Action       string  `json:"action"`
	Exchange     string  `json:"exchange"`
	PriceType    string  `json:"price_type"`
	Product      string  `json:"product"`
	Quantity     int     `json:"quantity"`
	Price        float64 `json:"price"`
	TriggerPrice float64 `json:"trigger_price"`
}

// SearchInstrumentsParams holds the query for instrument search.
type SearchInstrumentsParams struct {
	Query    string `json:"query"`
	Exchange string `json:"exchange"`
}

// GetQuoteParams holds the parameters for fetching a quote.
type GetQuoteParams struct {
	Symbol   string `json:"symbol"`
	Exchange string `json:"exchange"`
}
