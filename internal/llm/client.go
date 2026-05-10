package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/AgSpades/tele-trader.git/internal/broker"
	"github.com/AgSpades/tele-trader.git/internal/models"
)

const (
	// claudeModel is the exact model identifier used for all requests.
	claudeModel = "claude-sonnet-4-6"
	// maxToolLoopDepth prevents runaway tool-call chains.
	maxToolLoopDepth = 10
	// maxTokens is the maximum number of output tokens per Claude response.
	maxTokens = 4096
)

// Client orchestrates the Anthropic API with a tool-calling loop.
type Client struct {
	ac     anthropic.Client
	broker *broker.Client
}

// New creates a new LLM Client.
func New(apiKey string, brokerClient *broker.Client) *Client {
	ac := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Client{
		ac:     ac,
		broker: brokerClient,
	}
}

// ProcessSignal sends a cleaned Telegram signal to Claude and runs the tool-call
// execution loop until Claude produces a final text response or the depth guard triggers.
//
// The loop:
//  1. Send messages to Claude with tool definitions.
//  2. If stop_reason == "tool_use", dispatch each tool_use block to the broker.
//  3. Append tool_result blocks as a new "user" turn.
//  4. Repeat until stop_reason == "end_turn" or depth exceeded.
func (c *Client) ProcessSignal(ctx context.Context, signal models.Signal) (string, error) {
	slog.Info("llm: processing signal", "text", signal.CleanText)

	// Build the initial message history.
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(signal.CleanText)),
	}

	allTools := tools()

	for depth := 0; depth < maxToolLoopDepth; depth++ {
		slog.Debug("llm: calling claude", "depth", depth, "messages", len(messages))

		resp, err := c.ac.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(claudeModel),
			MaxTokens: maxTokens,
			System: []anthropic.TextBlockParam{
				{Text: MasterSystemPrompt},
			},
			Tools:    allTools,
			Messages: messages,
		})
		if err != nil {
			return "", fmt.Errorf("llm: claude api error (depth=%d): %w", depth, err)
		}

		slog.Debug("llm: claude response", "stop_reason", resp.StopReason, "depth", depth)

		// Append Claude's response to the conversation history.
		messages = append(messages, resp.ToParam())

		switch resp.StopReason {
		case anthropic.StopReasonEndTurn:
			// Extract the final text response.
			return extractText(resp), nil

		case anthropic.StopReasonToolUse:
			// Dispatch all tool_use blocks and collect results.
			toolResults, err := c.dispatchToolCalls(ctx, resp)
			if err != nil {
				return "", fmt.Errorf("llm: tool dispatch error (depth=%d): %w", depth, err)
			}

			// Append results as a new user turn.
			messages = append(messages, anthropic.NewUserMessage(toolResults...))

		default:
			return "", fmt.Errorf("llm: unexpected stop reason %q at depth %d", resp.StopReason, depth)
		}
	}

	return "", errors.New("llm: max tool loop depth exceeded — possible infinite loop")
}

// dispatchToolCalls iterates over all tool_use content blocks in a Claude response,
// calls the corresponding broker method, and returns a slice of tool_result blocks.
func (c *Client) dispatchToolCalls(ctx context.Context, resp *anthropic.Message) ([]anthropic.ContentBlockParamUnion, error) {
	var results []anthropic.ContentBlockParamUnion

	for _, block := range resp.Content {
		toolUse, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok {
			continue
		}

		slog.Info("llm: dispatching tool", "tool", toolUse.Name, "id", toolUse.ID)

		rawResult, execErr := c.executeTool(ctx, toolUse)

		var resultBlock anthropic.ContentBlockParamUnion
		if execErr != nil {
			slog.Error("llm: tool execution error", "tool", toolUse.Name, "error", execErr)
			errJSON, _ := json.Marshal(map[string]string{"status": "error", "message": execErr.Error()})
			resultBlock = anthropic.NewToolResultBlock(toolUse.ID, string(errJSON), true)
		} else {
			slog.Info("llm: tool result", "tool", toolUse.Name, "result_len", len(rawResult))
			resultBlock = anthropic.NewToolResultBlock(toolUse.ID, string(rawResult), false)
		}

		results = append(results, resultBlock)
	}

	return results, nil
}

// executeTool routes a Claude tool_use block to the appropriate broker method.
func (c *Client) executeTool(ctx context.Context, tool anthropic.ToolUseBlock) (json.RawMessage, error) {
	inputBytes, err := json.Marshal(tool.Input)
	if err != nil {
		return nil, fmt.Errorf("marshal tool input: %w", err)
	}

	switch tool.Name {
	case "search_instruments":
		var p models.SearchInstrumentsParams
		if err := json.Unmarshal(inputBytes, &p); err != nil {
			return nil, fmt.Errorf("parse search_instruments params: %w", err)
		}
		return c.broker.SearchInstruments(ctx, p)

	case "get_quote":
		var p models.GetQuoteParams
		if err := json.Unmarshal(inputBytes, &p); err != nil {
			return nil, fmt.Errorf("parse get_quote params: %w", err)
		}
		return c.broker.GetQuote(ctx, p)

	case "get_funds":
		return c.broker.GetFunds(ctx)

	case "place_order":
		var p models.PlaceOrderParams
		if err := json.Unmarshal(inputBytes, &p); err != nil {
			return nil, fmt.Errorf("parse place_order params: %w", err)
		}
		return c.broker.PlaceOrder(ctx, p)

	case "modify_order":
		var p models.ModifyOrderParams
		if err := json.Unmarshal(inputBytes, &p); err != nil {
			return nil, fmt.Errorf("parse modify_order params: %w", err)
		}
		return c.broker.ModifyOrder(ctx, p)

	case "get_position_book":
		return c.broker.GetPositionBook(ctx)

	default:
		return nil, fmt.Errorf("unknown tool %q", tool.Name)
	}
}

// extractText pulls the final text content block from a Claude response.
func extractText(resp *anthropic.Message) string {
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			return tb.Text
		}
	}
	return ""
}
