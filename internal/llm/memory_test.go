package llm

import (
	"encoding/json"
	"testing"
)

func TestUpsertTradeMemoryCreatesAndUpdates(t *testing.T) {
	t.Parallel()

	c := &Client{}

	raw := c.upsertTradeMemory(upsertTradeMemoryParams{
		Status:       tradeStatusPending,
		Index:        "nifty",
		Strike:       23800,
		OptionType:   "pe",
		Exchange:     "nfo",
		EntryTrigger: 182,
		StopLoss:     167,
		Targets:      []float64{193, 210, 230},
		LastMessage:  "NIFTY 23800 PE BUY ABOVE 182",
	})
	assertMemoryStatus(t, raw, "created")

	if len(c.tradeMemory) != 1 {
		t.Fatalf("tradeMemory length = %d, want 1", len(c.tradeMemory))
	}
	if got, want := c.tradeMemory[0].ID, "NIFTY-23800-PE"; got != want {
		t.Fatalf("ID = %q, want %q", got, want)
	}

	raw = c.upsertTradeMemory(upsertTradeMemoryParams{
		ID:          "NIFTY-23800-PE",
		Status:      tradeStatusActive,
		Symbol:      "NIFTY30MAY2623800PE",
		EntryPrice:  184,
		StopLoss:    182,
		TargetsHit:  1,
		Quantity:    75,
		SLOrderID:   "sl-123",
		LastMessage: "1ST TARGET DONE",
	})
	assertMemoryStatus(t, raw, "updated")

	got := c.tradeMemory[0]
	if got.Status != tradeStatusActive {
		t.Fatalf("Status = %q, want %q", got.Status, tradeStatusActive)
	}
	if got.Index != "NIFTY" || got.Strike != 23800 || got.OptionType != "PE" {
		t.Fatalf("identity fields were not preserved: %+v", got)
	}
	if got.TargetsHit != 1 || got.StopLoss != 182 || got.SLOrderID != "sl-123" {
		t.Fatalf("updated fields not applied: %+v", got)
	}
}

func TestCloseTradeMemory(t *testing.T) {
	t.Parallel()

	c := &Client{}
	c.upsertTradeMemory(upsertTradeMemoryParams{
		ID:         "SENSEX-74700-CE",
		Status:     tradeStatusActive,
		Index:      "SENSEX",
		Strike:     74700,
		OptionType: "CE",
		Symbol:     "SENSEX30MAY2674700CE",
	})

	raw := c.closeTradeMemory(closeTradeMemoryParams{
		Symbol: "SENSEX30MAY2674700CE",
		Reason: "profit booked",
	})
	assertMemoryStatus(t, raw, "closed")

	if got := c.tradeMemory[0].Status; got != tradeStatusClosed {
		t.Fatalf("Status = %q, want %q", got, tradeStatusClosed)
	}
	if got := c.tradeMemory[0].Notes; got != "profit booked" {
		t.Fatalf("Notes = %q, want profit booked", got)
	}
}

func assertMemoryStatus(t *testing.T, raw json.RawMessage, want string) {
	t.Helper()

	var got struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal memory result: %v", err)
	}
	if got.Status != want {
		t.Fatalf("status = %q, want %q; raw=%s", got.Status, want, string(raw))
	}
}
