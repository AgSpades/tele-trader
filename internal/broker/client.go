// Package broker wraps the OpenAlgo Go SDK, exposing typed methods matching the
// LLM tool definitions. All calls are guarded by a shared rate limiter.
package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/marketcalls/openalgo-go/openalgo"
	"golang.org/x/time/rate"

	"github.com/AgSpades/tele-trader.git/internal/models"
)

// apiStatus is used to inspect the top-level "status" field in OpenAlgo responses.
type apiStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

const (
	// DefaultStrategy is the strategy tag sent to OpenAlgo for all orders placed by the bot.
	DefaultStrategy = "TeleTrader"
	// DefaultProduct is the default product type (intraday).
	DefaultProduct = "MIS"
	// maxRPS is the maximum requests per second to OpenAlgo.
	maxRPS = 5
)

// Client wraps the openalgo SDK client with rate limiting and structured logging.
type Client struct {
	oa      *openalgo.Client
	limiter *rate.Limiter
}

// New creates a new broker Client.
func New(apiKey, host string) *Client {
	oa := openalgo.NewClient(apiKey, host)
	return &Client{
		oa:      oa,
		limiter: rate.NewLimiter(rate.Limit(maxRPS), maxRPS),
	}
}

// wait blocks until the rate limiter allows a request, respecting ctx cancellation.
func (c *Client) wait(ctx context.Context) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("broker: rate limiter: %w", err)
	}
	return nil
}

// marshal converts an OpenAlgo response to a raw JSON message for the LLM.
func marshal(v interface{}) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("broker: marshal response: %w", err)
	}
	return b, nil
}

// SearchInstruments searches for trading instruments matching a query string.
func (c *Client) SearchInstruments(ctx context.Context, p models.SearchInstrumentsParams) (json.RawMessage, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	slog.Info("broker: search_instruments", "query", p.Query, "exchange", p.Exchange)
	resp, err := c.oa.Search(p.Query, p.Exchange)
	if err != nil {
		return errJSON(err), nil // return error as JSON so LLM can reason about it
	}
	filtered := filterNearestExpiry(resp, p.Query, p.Exchange)
	return marshal(filtered)
}

func filterNearestExpiry(resp interface{}, query, exchange string) interface{} {
	upperExchange := strings.ToUpper(strings.TrimSpace(exchange))
	if upperExchange != "NFO" && upperExchange != "BFO" {
		return resp
	}

	upperQuery := strings.ToUpper(query)
	if !strings.Contains(upperQuery, " CE") && !strings.Contains(upperQuery, " PE") {
		return resp
	}

	payload, ok := toMap(resp)
	if !ok {
		return resp
	}

	data, ok := payload["data"].([]interface{})
	if !ok || len(data) == 0 {
		return resp
	}

	today := startOfDay(time.Now())
	var nearest time.Time
	for _, item := range data {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		expiry := strings.TrimSpace(stringValue(entry["expiry"]))
		if expiry == "" {
			continue
		}
		parsed, err := time.ParseInLocation("02-JAN-06", strings.ToUpper(expiry), today.Location())
		if err != nil {
			continue
		}
		parsed = startOfDay(parsed)
		if parsed.Before(today) {
			continue
		}
		if nearest.IsZero() || parsed.Before(nearest) {
			nearest = parsed
		}
	}

	if nearest.IsZero() {
		return resp
	}

	filtered := make([]interface{}, 0, len(data))
	for _, item := range data {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		expiry := strings.TrimSpace(stringValue(entry["expiry"]))
		if expiry == "" {
			continue
		}
		parsed, err := time.ParseInLocation("02-JAN-06", strings.ToUpper(expiry), today.Location())
		if err != nil {
			continue
		}
		if startOfDay(parsed).Equal(nearest) {
			filtered = append(filtered, entry)
		}
	}

	if len(filtered) == 0 {
		return resp
	}

	payload["data"] = filtered
	slog.Info("broker: search_instruments filtered", "exchange", exchange, "nearest_expiry", nearest.Format("2006-01-02"), "count", len(filtered))
	return payload
}

func toMap(resp interface{}) (map[string]interface{}, bool) {
	if resp == nil {
		return nil, false
	}
	if payload, ok := resp.(map[string]interface{}); ok {
		return payload, true
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return nil, false
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, false
	}
	return payload, true
}

func startOfDay(t time.Time) time.Time {
	loc := t.Location()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func stringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch typed := v.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', 0, 64)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

// GetQuote fetches the current quote (LTP, bid, ask, etc.) for a symbol.
func (c *Client) GetQuote(ctx context.Context, p models.GetQuoteParams) (json.RawMessage, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	slog.Info("broker: get_quote", "symbol", p.Symbol, "exchange", p.Exchange)
	resp, err := c.oa.Quotes(p.Symbol, p.Exchange)
	if err != nil {
		return errJSON(err), nil
	}
	return marshal(resp)
}

// GetFunds returns the available margin and cash balance.
func (c *Client) GetFunds(ctx context.Context) (json.RawMessage, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	slog.Info("broker: get_funds")
	resp, err := c.oa.Funds()
	if err != nil {
		return errJSON(err), nil
	}
	return marshal(resp)
}

// Ping verifies that the OpenAlgo server is reachable and the API key is valid.
// It calls the Funds endpoint and treats any "status": "error" response as a
// connectivity failure. Used by the startup health-check gate.
func (c *Client) Ping(ctx context.Context) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("broker: ping rate limiter: %w", err)
	}
	resp, err := c.oa.Funds()
	if err != nil {
		return fmt.Errorf("broker: ping: %w", err)
	}
	// Marshal and inspect the status field.
	b, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("broker: ping marshal: %w", err)
	}
	var s apiStatus
	if err := json.Unmarshal(b, &s); err == nil && s.Status == "error" {
		return fmt.Errorf("broker: ping: OpenAlgo API error: %s", s.Message)
	}
	return nil
}

// PlaceOrder places a new order. Supports MARKET, LIMIT, and SL order types.
// For SL orders, both Price and TriggerPrice must be set (Zerodha does not support SL-M on options).
func (c *Client) PlaceOrder(ctx context.Context, p models.PlaceOrderParams) (json.RawMessage, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}

	// Apply defaults.
	if p.Strategy == "" {
		p.Strategy = DefaultStrategy
	}
	if p.Product == "" {
		p.Product = DefaultProduct
	}

	slog.Info("broker: place_order",
		"symbol", p.Symbol,
		"action", p.Action,
		"exchange", p.Exchange,
		"price_type", p.PriceType,
		"quantity", p.Quantity,
		"price", p.Price,
		"trigger_price", p.TriggerPrice,
	)

	opts := map[string]interface{}{}
	if p.Price != 0 {
		opts["price"] = strconv.FormatFloat(p.Price, 'f', 2, 64)
	}
	if p.TriggerPrice != 0 {
		opts["trigger_price"] = strconv.FormatFloat(p.TriggerPrice, 'f', 2, 64)
	}

	var resp interface{}
	var err error
	if len(opts) > 0 {
		resp, err = c.oa.PlaceOrder(p.Strategy, p.Symbol, p.Action, p.Exchange, p.PriceType, p.Product, p.Quantity, opts)
	} else {
		resp, err = c.oa.PlaceOrder(p.Strategy, p.Symbol, p.Action, p.Exchange, p.PriceType, p.Product, p.Quantity)
	}
	if err != nil {
		return errJSON(err), nil
	}
	return marshal(resp)
}

// ModifyOrder updates an existing SL order (e.g., trailing the stop-loss).
func (c *Client) ModifyOrder(ctx context.Context, p models.ModifyOrderParams) (json.RawMessage, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}

	if p.Strategy == "" {
		p.Strategy = DefaultStrategy
	}
	if p.Product == "" {
		p.Product = DefaultProduct
	}

	slog.Info("broker: modify_order",
		"order_id", p.OrderID,
		"symbol", p.Symbol,
		"trigger_price", p.TriggerPrice,
		"price", p.Price,
	)

	resp, err := c.oa.ModifyOrder(
		p.OrderID,
		p.Strategy,
		p.Symbol,
		p.Action,
		p.Exchange,
		p.PriceType,
		p.Product,
		p.Quantity,
		strconv.FormatFloat(p.Price, 'f', 2, 64),
		"0", // disclosed quantity
		strconv.FormatFloat(p.TriggerPrice, 'f', 2, 64),
	)
	if err != nil {
		return errJSON(err), nil
	}
	return marshal(resp)
}

// CancelOrder cancels an existing pending order, typically an old protective SL.
func (c *Client) CancelOrder(ctx context.Context, p models.CancelOrderParams) (json.RawMessage, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}

	if p.Strategy == "" {
		p.Strategy = DefaultStrategy
	}

	slog.Info("broker: cancel_order", "order_id", p.OrderID, "strategy", p.Strategy)

	resp, err := c.oa.CancelOrder(p.OrderID, p.Strategy)
	if err != nil {
		return errJSON(err), nil
	}
	return marshal(resp)
}

// GetPositionBook returns the current open positions.
func (c *Client) GetPositionBook(ctx context.Context) (json.RawMessage, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	slog.Info("broker: get_position_book")
	resp, err := c.oa.PositionBook()
	if err != nil {
		return errJSON(err), nil
	}
	return marshal(resp)
}

// GetOrderBook returns the current active and completed orders for the day.
func (c *Client) GetOrderBook(ctx context.Context) (json.RawMessage, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	slog.Info("broker: get_order_book")
	resp, err := c.oa.OrderBook()
	if err != nil {
		return errJSON(err), nil
	}
	return marshal(resp)
}

// errJSON wraps an error as a JSON object so the LLM can handle it gracefully.
func errJSON(err error) json.RawMessage {
	type errResp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	b, _ := json.Marshal(errResp{Status: "error", Message: err.Error()})
	return b
}
