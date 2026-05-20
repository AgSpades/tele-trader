package llm

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// tools returns the full list of Anthropic tool definitions exposed to Claude.
// Most tools call the broker; trade-memory tools update local structured state.
func tools() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		anthropic.ToolUnionParamOfTool(
			anthropic.ToolInputSchemaParam{
				Type: "object",
				Properties: map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Natural language query, e.g. 'SENSEX 77200 PE' or 'NIFTY 26000 CE DEC'",
					},
					"exchange": map[string]interface{}{
						"type":        "string",
						"description": "Exchange to search. Use 'NFO' for Nifty options, 'BFO' for Sensex/Bankex options.",
						"enum":        []string{"NFO", "BFO", "NSE", "BSE", "MCX"},
					},
				},
				Required: []string{"query", "exchange"},
			},
			"search_instruments",
		),
		anthropic.ToolUnionParamOfTool(
			anthropic.ToolInputSchemaParam{
				Type: "object",
				Properties: map[string]interface{}{
					"symbol": map[string]interface{}{
						"type":        "string",
						"description": "Exact trading symbol, e.g. 'NIFTY30DEC2526000CE'",
					},
					"exchange": map[string]interface{}{
						"type":        "string",
						"description": "Exchange the symbol belongs to, e.g. 'NFO' or 'BFO'",
					},
				},
				Required: []string{"symbol", "exchange"},
			},
			"get_quote",
		),
		anthropic.ToolUnionParamOfTool(
			anthropic.ToolInputSchemaParam{
				Type:       "object",
				Properties: map[string]interface{}{},
				Required:   []string{},
			},
			"get_funds",
		),
		anthropic.ToolUnionParamOfTool(
			anthropic.ToolInputSchemaParam{
				Type: "object",
				Properties: map[string]interface{}{
					"symbol": map[string]interface{}{
						"type":        "string",
						"description": "Exact trading symbol returned by search_instruments",
					},
					"action": map[string]interface{}{
						"type":        "string",
						"description": "Order direction",
						"enum":        []string{"BUY", "SELL"},
					},
					"exchange": map[string]interface{}{
						"type":        "string",
						"description": "Exchange, e.g. NFO or BFO",
					},
					"price_type": map[string]interface{}{
						"type":        "string",
						"description": "Order type. Use MARKET for entries, SL for stop-loss orders. NOTE: Zerodha does NOT support SL-M orders for NFO/BFO options.",
						"enum":        []string{"MARKET", "LIMIT", "SL"},
					},
					"product": map[string]interface{}{
						"type":        "string",
						"description": "Product type. MIS for intraday (default), NRML for positional.",
						"enum":        []string{"MIS", "NRML"},
					},
					"quantity": map[string]interface{}{
						"type":        "integer",
						"description": "Number of units (not lots). Must be a multiple of lot_size.",
					},
					"price": map[string]interface{}{
						"type":        "number",
						"description": "Limit price. Required for LIMIT and SL orders. For protective SELL SL orders: set 2-3 points BELOW trigger_price.",
					},
					"trigger_price": map[string]interface{}{
						"type":        "number",
						"description": "Trigger price for SL orders. Required when price_type is SL. The order activates when LTP crosses this.",
					},
					"strategy": map[string]interface{}{
						"type":        "string",
						"description": "Strategy name tag. Defaults to TeleTrader if omitted.",
					},
				},
				Required: []string{"symbol", "action", "exchange", "price_type", "quantity"},
			},
			"place_order",
		),
		anthropic.ToolUnionParamOfTool(
			anthropic.ToolInputSchemaParam{
				Type: "object",
				Properties: map[string]interface{}{
					"order_id": map[string]interface{}{
						"type":        "string",
						"description": "The order ID returned when the SL order was placed",
					},
					"symbol": map[string]interface{}{
						"type":        "string",
						"description": "Same symbol as the original order",
					},
					"action": map[string]interface{}{
						"type":        "string",
						"description": "Same action as the original order (typically SELL for SL)",
						"enum":        []string{"BUY", "SELL"},
					},
					"exchange": map[string]interface{}{
						"type": "string",
					},
					"price_type": map[string]interface{}{
						"type": "string",
					},
					"product": map[string]interface{}{
						"type": "string",
					},
					"quantity": map[string]interface{}{
						"type": "integer",
					},
					"price": map[string]interface{}{
						"type":        "number",
						"description": "New limit price, usually 2-3 points below the new trigger for a protective SELL SL order.",
					},
					"trigger_price": map[string]interface{}{
						"type":        "number",
						"description": "New trigger price for the trailed stop-loss",
					},
					"strategy": map[string]interface{}{
						"type": "string",
					},
				},
				Required: []string{"order_id", "symbol", "action", "exchange", "price_type", "quantity", "price", "trigger_price"},
			},
			"modify_order",
		),
		anthropic.ToolUnionParamOfTool(
			anthropic.ToolInputSchemaParam{
				Type: "object",
				Properties: map[string]interface{}{
					"order_id": map[string]interface{}{
						"type":        "string",
						"description": "The pending order ID to cancel, usually the active protective SELL SL order after a manual/profit-book exit.",
					},
					"strategy": map[string]interface{}{
						"type":        "string",
						"description": "Strategy name tag. Defaults to TeleTrader if omitted.",
					},
				},
				Required: []string{"order_id"},
			},
			"cancel_order",
		),
		anthropic.ToolUnionParamOfTool(
			anthropic.ToolInputSchemaParam{
				Type:       "object",
				Properties: map[string]interface{}{},
				Required:   []string{},
			},
			"get_position_book",
		),
		anthropic.ToolUnionParamOfTool(
			anthropic.ToolInputSchemaParam{
				Type:       "object",
				Properties: map[string]interface{}{},
				Required:   []string{},
			},
			"get_order_book",
		),
		anthropic.ToolUnionParamOfTool(
			anthropic.ToolInputSchemaParam{
				Type: "object",
				Properties: map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Stable trade memory ID. Use symbol when known, otherwise INDEX-STRIKE-CE/PE, e.g. NIFTY-23800-PE.",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"description": "Current lifecycle state for this call.",
						"enum":        []string{"pending", "active", "closed"},
					},
					"index": map[string]interface{}{
						"type":        "string",
						"description": "Underlying index, e.g. NIFTY or SENSEX.",
					},
					"strike": map[string]interface{}{
						"type":        "integer",
						"description": "Option strike price.",
					},
					"option_type": map[string]interface{}{
						"type":        "string",
						"description": "Option side.",
						"enum":        []string{"CE", "PE"},
					},
					"exchange": map[string]interface{}{
						"type":        "string",
						"description": "Exchange for the option, e.g. NFO or BFO.",
					},
					"symbol": map[string]interface{}{
						"type":        "string",
						"description": "Resolved broker symbol when known.",
					},
					"entry_trigger": map[string]interface{}{
						"type":        "number",
						"description": "BUY ABOVE trigger or entry reference.",
					},
					"entry_upper": map[string]interface{}{
						"type":        "number",
						"description": "Upper edge of entry range. Use same value as entry_trigger for single-price entries.",
					},
					"entry_price": map[string]interface{}{
						"type":        "number",
						"description": "Actual/average entry price once active.",
					},
					"stop_loss": map[string]interface{}{
						"type":        "number",
						"description": "Current SL trigger level.",
					},
					"targets": map[string]interface{}{
						"type":        "array",
						"description": "Known target levels in ascending order.",
						"items":       map[string]interface{}{"type": "number"},
					},
					"targets_hit": map[string]interface{}{
						"type":        "integer",
						"description": "Number of targets completed so far.",
					},
					"quantity": map[string]interface{}{
						"type":        "integer",
						"description": "Open quantity in units, not lots.",
					},
					"sl_order_id": map[string]interface{}{
						"type":        "string",
						"description": "Protective SELL SL order ID when known.",
					},
					"last_message": map[string]interface{}{
						"type":        "string",
						"description": "Most recent Telegram message that changed this memory.",
					},
					"notes": map[string]interface{}{
						"type":        "string",
						"description": "Short context note, e.g. pending trigger waiting or first target hit.",
					},
				},
				Required: []string{"status"},
			},
			"upsert_trade_memory",
		),
		anthropic.ToolUnionParamOfTool(
			anthropic.ToolInputSchemaParam{
				Type: "object",
				Properties: map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Trade memory ID to close.",
					},
					"symbol": map[string]interface{}{
						"type":        "string",
						"description": "Resolved broker symbol to close if ID is unavailable.",
					},
					"reason": map[string]interface{}{
						"type":        "string",
						"description": "Short reason for closing, e.g. profit booked or all targets done.",
					},
				},
			},
			"close_trade_memory",
		),
	}
}
