# Project / Conversation Title

Tele-Trader: LLM-Driven Telegram Options Execution Bot

# Objective

The user is building a Go service that listens to a private Telegram trading channel, understands noisy Indian index option calls with an LLM, and automates trade execution/management through OpenAlgo/Zerodha.

Explicit goals:

- Parse Telegram trade calls for NIFTY and SENSEX.
- Execute valid option entries safely through OpenAlgo.
- Use channel-provided SL when present, otherwise configured fallback SL.
- Trail stop losses and exit/cancel protective orders when targets or profit-book messages arrive.
- Preserve context across fragmented Telegram messages.
- Avoid missing fast signals when many channel messages arrive together.
- Write logs to daily files.

Implicit goals:

- Reduce brittle regex logic by letting Claude handle unstructured signal language.
- Keep execution safe: no duplicate positions, no chasing, no SL-M for Zerodha options.
- Improve production readiness: bounded loops, structured memory, logs, tests, concurrency controls.
- Move toward live-price-driven trade management rather than relying only on Telegram numeric ticks.

# High-Level Summary

- Repo is `/home/agspades/projects/tele-trader`.
- Project is a Go Telegram userbot that reads trading channel messages and lets Claude operate broker tools through OpenAlgo.
- Original prompt was generic/Sensex-oriented; it was updated for the new `ZERO TO HERO PREMIUM` style messages.
- Channel examples include NIFTY and SENSEX option calls, bare numeric price updates, target-done updates, jokes/commentary, and profit-book messages.
- Prompt now supports NIFTY/NIFTY 50/NIFTY50 -> NFO and SENSEX -> BFO, plus BANKNIFTY/BANKEX mappings.
- Added `cancel_order` broker/tool support so protective SL orders can be cancelled after manual/profit-book exits.
- Added structured in-process trade memory (`TradeMemory`) separate from short chat history.
- Added internal LLM tools: `upsert_trade_memory` and `close_trade_memory`.
- Added daily JSON file logging to `logs/DDMMYYYY.log` alongside console logging.
- Analysis of real log `logs/18052026.log` showed the serial processor was too slow: many signals arrived at the same timestamp but were processed one by one over many minutes.
- Added Telegram burst batching: messages within `900ms` are joined into one LLM job, max `12` lines.
- Added `3` signal workers so slow LLM calls do not block later independent jobs.
- Relaxed `llm.Client` locking so shared memory/history are locked only for snapshots/mutations, not the full Claude/tool loop.
- Added quote monitor polling every `2s` for pending/active trades; it emits internal `LIVE_QUOTE_UPDATE` events when pending triggers or active targets are hit.
- Tests pass with `go test ./... -count=1`.

# User Intent & Preferences

- Wants practical code changes, not just theory.
- Likes explanations in junior-developer-friendly terms when requested.
- Wants production-grade behavior, especially around trade safety and missed-signal avoidance.
- Values reliable execution timing for fast Telegram signal bursts.
- Prefers direct iteration: identify issue from logs, implement improvement, run tests.
- Wants concise final summaries with validation status.
- Is comfortable with LLM-driven architecture but expects guardrails and clear failure modes.
- Uses skills explicitly, especially `golang-pro` for Go/concurrency and previously `ai-agents-architect` for agent behavior.
- Wants logs preserved after process exit.
- Wants generated build binary `bin/tele-trader` not committed; `bin/*` is ignored.

# Current State

Completed work:

- Updated LLM system prompt for new Telegram channel style.
- Added SENSEX support and exchange mapping.
- Added `cancel_order` tool through models, tool schema, LLM dispatch, and broker wrapper.
- Added Telegram channel ID normalization for `-100...` IDs.
- Added bounded LLM chat memory (`maxMemoryMessages = 24`).
- Added structured trade memory in `internal/llm/memory.go`.
- Added `upsert_trade_memory` and `close_trade_memory`.
- Added tests for trade memory create/update/close behavior.
- Added daily file logging under `logs/`, format `DDMMYYYY.log`.
- Added tests for logging.
- Added signal burst batcher and worker pool in `cmd/bot/processor.go`.
- Added quote monitor in `cmd/bot/quote_monitor.go`.
- Added tests for quote LTP extraction.
- Updated prompt for batched messages, `ABV`, and internal `LIVE_QUOTE_UPDATE` events.

Current implementation state:

- Telegram messages go into `msgCh`.
- `runSignalBatcher` groups messages within `900ms` or until `12` messages.
- Batches go into `signalProcessor.jobs`.
- `3` workers call `llm.Client.ProcessSignal`.
- `llm.Client` snapshots trade memory/history, sends Claude `TRADING_MEMORY` + `TELEGRAM_MESSAGE`, and lets Claude call tools.
- `quoteMonitor` polls broker quotes every `2s` for pending/active trades in `TradeMemory`.
- Quote monitor emits synthetic `LIVE_QUOTE_UPDATE` messages into the same signal processor.

Latest outputs:

- `go test ./... -count=1` passes.
- Logs from a real run are in `logs/18052026.log`.

What currently works:

- Startup health checks for Telegram and OpenAlgo.
- Telegram auth with OTP/2FA.
- Daily JSON logging to file and stdout.
- Claude tool orchestration with max tool loop depth.
- Broker tools: search, quote, funds, place, modify, cancel, position book, order book.
- Structured memory updates via LLM tools.
- Burst batching and multiple workers.
- Quote monitor event generation.

Known broken or risky areas:

- Quote monitor is polling-based, not websocket streaming.
- Quote monitor emits events into LLM rather than directly executing; there is still LLM latency after trigger detection.
- `signalProcessor.Stop()` waits for workers to exit via context; ensure it is only called after context cancellation.
- Concurrent LLM workers can operate from slightly stale snapshots of trade memory. Memory mutations are locked, but decisions may be based on older snapshots.
- Full trade-manager actor has not been implemented yet. Current architecture is an intermediate step.
- `TradeMemory` is in-process only; restart loses memory and must recover from broker position/order books.
- Closed/pending stale trades may remain in memory up to `maxTradeMemory = 32`.
- No explicit quote monitor unit test for `eventForTrade`; only LTP extraction is tested.

# Important Decisions Made

- Decision: Use LLM prompt/tool orchestration instead of regex parsing for signal meaning.
- Why: Telegram signals are noisy, informal, multilingual, and vary in format.
- Alternatives rejected: Pure regex parser; too brittle for messages like `ABV`, fragmented target/SL lines, emojis, and bare numbers.

- Decision: Use channel-provided SL when valid; fallback to config percentage only when SL missing.
- Why: The channel gives explicit trade management levels, and user asked this directly.
- Alternatives rejected: Always using config `OPENING_SL_PERCENT`; would ignore channel risk rules.

- Decision: Add `cancel_order`.
- Why: If the bot exits a position with market SELL, a pending protective SL can remain and later cause unintended sell/short behavior.
- Alternatives rejected: Only placing market exit and assuming broker clears SL; unsafe.

- Decision: Add structured `TradeMemory`.
- Why: `maxMemoryMessages = 24` means chat history only covers about 12 Telegram messages; active/pending trades can outlive that.
- Alternatives rejected: Increasing chat history indefinitely; expensive, stale, and unreliable.

- Decision: Keep `TradeMemory` in-process for now.
- Why: Quick production improvement without storage design complexity.
- Alternatives rejected: Immediate database/disk persistence; should be next hardening step but was not required to unblock.

- Decision: Add daily file logging with JSON `slog`.
- Why: Console logs vanish after bot exits; user wanted `DDMMYYYY.log` under `logs/`.
- Alternatives rejected: Plain text logs; current app already uses structured JSON logs.

- Decision: Batch Telegram bursts for `900ms`.
- Why: Channel often sends entry/target/SL/wait lines back-to-back; batching lets Claude see full context in one turn and avoids wasting separate LLM calls.
- Alternatives rejected: Immediate per-message LLM calls; observed to cause queue lag and missed context.

- Decision: Add bounded worker pool with `3` workers.
- Why: A single slow LLM call blocked later signals for minutes in logs.
- Alternatives rejected: Unlimited goroutine per message; unsafe for broker/memory and API cost.

- Decision: Relax `llm.Client` mutex scope.
- Why: Holding the lock during the entire Claude/tool loop serialized all workers and defeated concurrency.
- Alternatives rejected: Removing locks entirely; unsafe for memory/history.

- Decision: Add polling quote monitor.
- Why: Pending entries/targets should not depend only on Telegram numeric ticks. Live prices should drive trigger/target events.
- Alternatives rejected: Websocket streaming for this pass; more complex and not yet necessary to prove architecture.

# Technical Context

## Stack

- Language: Go.
- Module: `github.com/AgSpades/tele-trader.git`.
- Go version in `go.mod`: `go 1.25.9`.
- Telegram client: `gotd/td`.
- LLM SDK: `github.com/anthropics/anthropic-sdk-go`.
- Broker SDK: `github.com/marketcalls/openalgo-go/openalgo`.
- Logging: Go `log/slog` JSON handlers.

## Architecture

Current flow:

```text
Telegram gotd handler
  -> msgCh
  -> runSignalBatcher (900ms / 12 msg max)
  -> signalProcessor jobs
  -> 3 workers
  -> llm.Client.ProcessSignal
  -> Claude tool loop
  -> broker tools + memory tools

quoteMonitor
  -> every 2s SnapshotTrades
  -> broker.GetQuote
  -> if trigger/target hit, Enqueue LIVE_QUOTE_UPDATE
```

LLM tool loop:

- Builds message with:

```text
TRADING_MEMORY:
<current structured JSON>

TELEGRAM_MESSAGE:
<cleaned message or newline batch>
```

- Claude can call broker tools and local memory tools.
- Tool loop max depth is now `24`.

## APIs

Broker tools exposed to Claude:

- `search_instruments`
- `get_quote`
- `get_funds`
- `place_order`
- `modify_order`
- `cancel_order`
- `get_position_book`
- `get_order_book`

Local/internal tools exposed to Claude:

- `upsert_trade_memory`
- `close_trade_memory`

## Libraries

Important imports:

- `github.com/anthropics/anthropic-sdk-go`
- `github.com/anthropics/anthropic-sdk-go/option`
- `github.com/marketcalls/openalgo-go/openalgo`
- `github.com/gotd/td/telegram`
- `github.com/gotd/td/tg`
- `golang.org/x/time/rate`
- `log/slog`

## File structure

Important files:

- `cmd/bot/main.go`: app startup, wiring, health checks, signal processor/quote monitor startup.
- `cmd/bot/processor.go`: burst batching, worker pool, per-signal LLM call wrapper.
- `cmd/bot/quote_monitor.go`: polling live quotes and emitting internal events.
- `cmd/bot/logging.go`: stdout + daily file logger.
- `internal/llm/client.go`: Claude tool loop and dispatch.
- `internal/llm/prompt.go`: system prompt.
- `internal/llm/tools.go`: Anthropic tool schemas.
- `internal/llm/memory.go`: structured in-process trade memory.
- `internal/broker/client.go`: OpenAlgo wrapper.
- `internal/models/types.go`: shared tool parameter types.
- `internal/telegram/handler.go`: Telegram userbot and message cleaning/filtering.
- `logs/18052026.log`: user-provided real run logs.

## Environment

From README/config:

- `TELEGRAM_APP_ID`
- `TELEGRAM_APP_API_HASH`
- `TELEGRAM_PHONE`
- `TELEGRAM_CHANNEL_ID`
- `ANTHROPIC_API_KEY`
- `OPENALGO_API_KEY`
- `OPENALGO_URL`, default `http://127.0.0.1:5000`
- `SESSION_FILE_PATH`, default `session.json`
- `DRY_RUN`, default `true`
- `TRADE_LOT_SIZE`, default `1`
- `OPENING_SL_PERCENT`, default `15.0`
- `STARTUP_CHECK_TIMEOUT_SEC`, default 120 seconds unless fresh Telegram login

## Commands

Used successfully:

```bash
gofmt -w <files>
go test ./... -count=1
```

Makefile commands:

```bash
make run
make build
make test
make docker-build
make docker-run
```

Note: Go is installed via Snap in this environment; sandboxed `go`/`gofmt` initially failed with `snap-confine` errors, requiring escalated execution.

## Versions

- Current date in environment: 2026-05-18.
- User timezone in env context: Asia/Kolkata.
- Claude model constant: `claude-sonnet-4-6`.
- `go.mod`: Go `1.25.9`.

## Dependencies

See `go.mod` and `go.sum`. Important direct dependencies include Anthropic SDK, gotd Telegram, OpenAlgo SDK, godotenv, and rate limiter.

## Data structures

`TradeMemory` in `internal/llm/memory.go`:

```go
type TradeMemory struct {
    ID           string
    Status       string // pending, active, closed
    Index        string
    Strike       int
    OptionType   string
    Exchange     string
    Symbol       string
    EntryTrigger float64
    EntryUpper   float64
    EntryPrice   float64
    StopLoss     float64
    Targets      []float64
    TargetsHit   int
    Quantity     int
    SLOrderID    string
    LastMessage  string
    Notes        string
    UpdatedAt    time.Time
}
```

Important constants:

```go
maxMemoryMessages = 24
maxTradeMemory = 32
maxToolLoopDepth = 24
signalBatchWindow = 900 * time.Millisecond
signalBatchMax = 12
signalWorkers = 3
signalTimeout = 5 * time.Minute
quoteMonitorInterval = 2 * time.Second
```

# Important Entities

People:

- User: building and testing Tele-Trader, wants reliable automated execution.

Projects:

- Tele-Trader: AI-powered Telegram userbot for Indian derivatives execution.

Repos:

- Local repo: `/home/agspades/projects/tele-trader`.

Files:

- `internal/llm/prompt.go`: central behavior contract for Claude.
- `internal/llm/client.go`: Claude loop, memory snapshot injection, tool dispatch.
- `internal/llm/memory.go`: structured trade state.
- `internal/llm/tools.go`: tool schemas.
- `internal/broker/client.go`: OpenAlgo methods.
- `cmd/bot/processor.go`: batching/concurrent workers.
- `cmd/bot/quote_monitor.go`: live quote event generation.
- `cmd/bot/logging.go`: daily file logging.
- `internal/telegram/handler.go`: Telegram ingestion and cleaning.
- `.gitignore`: already ignores `.env`, `*.log`, `bin/*`.
- `logs/18052026.log`: evidence of old serial processing bottleneck.

Variables:

- `TRADE_LOT_SIZE`: number of lots to execute.
- `OPENING_SL_PERCENT`: fallback SL percent when channel does not provide SL.
- `DRY_RUN`: defaults true; routes orders through OpenAlgo Analyzer.
- `TELEGRAM_CHANNEL_ID`: channel filter; handler normalizes `-100...`.

Components:

- Telegram Handler: receives and cleans channel messages.
- Signal Batcher: groups message bursts.
- Signal Processor: worker pool for LLM processing.
- LLM Client: Claude orchestration and memory tool handling.
- Broker Client: OpenAlgo wrapper.
- Quote Monitor: polls active/pending symbols and emits `LIVE_QUOTE_UPDATE`.
- Logger: stdout + daily JSON file.

Services:

- Telegram.
- Anthropic Claude API.
- OpenAlgo.
- Zerodha through OpenAlgo.

Tools:

- `gofmt`
- `go test`
- `rg`
- `apply_patch`

# Constraints and Non-Negotiables

- Must not use Zerodha SL-M for NFO/BFO options.
- Protective stop losses must be SL orders with `trigger_price` and `price`.
- For protective SELL SL, price should be 2-3 points below trigger.
- NIFTY/NIFTY 50/NIFTY50/BANKNIFTY -> NFO.
- SENSEX/BANKEX -> BFO.
- Must avoid duplicate positions for same symbol.
- Must not execute incomplete or ambiguous signals.
- Must not chase entries more than 5% above upper entry reference.
- Must prefer channel-provided SL when valid.
- Must cancel protective SL after manual/profit-book full exit.
- Must preserve user changes; do not revert unrelated work.
- Must not commit generated binary `bin/tele-trader`; `bin/*` ignored.
- `.env` must not be committed.
- Logs are ignored via `*.log`.
- Keep tool loops bounded.
- Broker/API calls must respect rate limiter in broker client.

# Open Problems

Problem: Quote monitor is polling, not streaming.
Attempted fixes: Added `quoteMonitor` polling every `2s`.
Observed behavior: Not yet tested in live market after implementation.
Suspected causes: Simpler implementation chosen; OpenAlgo SDK has websocket support, but not yet integrated.

Problem: LLM still makes execution decisions after live quote event.
Attempted fixes: Quote monitor emits `LIVE_QUOTE_UPDATE` to reduce dependency on Telegram ticks.
Observed behavior: There is still Claude latency between quote trigger and order placement.
Suspected causes: Architecture still routes decisions through LLM rather than deterministic trade manager.

Problem: Concurrent LLM workers may use stale trade memory snapshots.
Attempted fixes: Memory mutations are locked; snapshots are copied before LLM call.
Observed behavior: Safe from data races, but logic races possible.
Suspected causes: No single trade-manager actor owns decisions yet.

Problem: In-process trade memory is lost on restart.
Attempted fixes: LLM can query `position_book`/`order_book`; no persistence added.
Observed behavior: After restart, semantic context like targets may be missing.
Suspected causes: No disk/database persistence implemented.

Problem: Stale pending trades can remain in memory.
Attempted fixes: `close_trade_memory` can mark stale pending closed when all targets done/missed.
Observed behavior: Real log showed pending calls staying while later target updates arrived.
Suspected causes: No expiry/TTL cleanup policy.

Problem: LLM may close memory for never-entered pending calls on all-target messages.
Attempted fixes: Prompt says no position -> ignore, but later implementation also allows closing stale pending.
Observed behavior: In log, `130+++ ALL TARGET DONE` closed `NIFTY-23350-PE` even though never entered.
Suspected causes: Ambiguous prompt around clearing stale pending vs leaving it pending.

Problem: Shutdown can cancel an in-flight LLM call.
Attempted fixes: Per-signal context uses parent ctx and 5-minute timeout.
Observed behavior: Log shows `context canceled` for `750++++ ALL TARGET DONE` on shutdown.
Suspected causes: Expected when process is terminated while work is in flight.

# Pending Tasks

Priority: High
Task: Implement a true TradeManager actor.
Notes: One goroutine should own trade state and broker side effects. LLM should produce structured intents, not directly manage execution across concurrent calls.

Priority: High
Task: Add deterministic parser/fast path for clear entry calls.
Notes: For messages like `BUY NIFTY 50 23050 PE ABOVE 28 TARGET 35-45+++ SL 21`, extract fields without LLM where possible, then let LLM handle ambiguous cases.

Priority: High
Task: Add quote-monitor execution path or trade-manager events.
Notes: Current quote monitor emits LLM messages. Better: quote monitor sends `TriggerHit`/`TargetHit` to TradeManager, which acts deterministically.

Priority: High
Task: Persist `TradeMemory`.
Notes: Save to JSON or SQLite under non-committed state directory. Reload on startup and reconcile with broker books.

Priority: Medium
Task: Add TTL/stale cleanup for pending calls.
Notes: Pending missed calls should expire after time/session/market event.

Priority: Medium
Task: Add tests for `quoteMonitor.eventForTrade`.
Notes: Cover pending trigger inside range, above chase limit, active target hit, no target.

Priority: Medium
Task: Make processor constants configurable.
Notes: `signalBatchWindow`, `signalWorkers`, `quoteMonitorInterval` may need tuning.

Priority: Medium
Task: Add structured logs for memory snapshots and tool inputs, carefully redacting secrets.
Notes: Current logs include agent response and tool names; deeper observability useful for post-trade audit.

Priority: Medium
Task: Add OpenAlgo websocket support.
Notes: SDK appears to support websocket LTP subscriptions. Could reduce latency vs 2-second polling.

Priority: Low
Task: Add README updates for new architecture and logs.
Notes: Document `logs/DDMMYYYY.log`, burst batching, quote monitor, memory behavior.

# Conversation Insights

- The user’s channel sends many back-to-back messages; treating them as isolated messages is wrong.
- The first live log showed a serious bottleneck: messages arrived at `09:17:59` but were processed sequentially over many minutes.
- Bare Telegram numeric updates are unreliable as trigger confirmation; live broker quote must be checked.
- LLM can reason well about noisy messages, but it is too slow to be the only real-time mechanism.
- Batching entry + target + SL fragments is a high-leverage improvement.
- Concurrency must be bounded. Unlimited parallel LLM/tool calls would risk duplicate trades.
- `llm.Client` originally locked across the whole Claude loop; this made worker concurrency ineffective.
- File logs immediately proved useful for diagnosing timing behavior.
- `ABV` appeared in live signal and needed prompt support.
- `SENSEX` support must use BFO.
- `DRY_RUN=true` appeared in logs; test execution likely went through Analyzer.
- The LLM sometimes chose a nearer/current weekly expiry after searching and quoting multiple symbols.

DO NOT REPEAT:

- Do not rely on `maxMemoryMessages` for trade context.
- Do not process Telegram bursts as unrelated isolated messages.
- Do not let one LLM call block all subsequent messages.
- Do not execute just because Telegram sends a bare number equal to a trigger; verify live LTP.
- Do not leave protective SL orders open after full exit.
- Do not make broker side effects from uncontrolled parallel goroutines without ownership/serialization.
- Do not assume NIFTY only; channel may send SENSEX too.

# Exact Artifacts

Key signal examples from user:

```text
BUY NIFTY 50
23200 PE
ABOVE 30
TARGET 40-45+++
SL 22
```

```text
NIfty 23750 CE
Buy above 190
Tgt 198/213/235+
Sl 180
```

```text
NIFTY 23800 PE
BUY ABOVE 182
TGT 193/210/230+
SL 167
```

```text
BUY NIFTY 23800 CE 130
SL 100
Target 150-180-260
```

Live log cluster that exposed serial bottleneck:

```json
{"time":"2026-05-18T09:17:59.123920835+05:30","level":"INFO","msg":"telegram: new signal received","raw_len":29,"clean":"NIFTY 23500 PE BUY ABOVE 160"}
{"time":"2026-05-18T09:17:59.124049834+05:30","level":"INFO","msg":"telegram: new signal received","raw_len":24,"clean":"Tgt 170/188/220+ Sl 142"}
{"time":"2026-05-18T09:17:59.124079657+05:30","level":"INFO","msg":"telegram: new signal received","raw_len":16,"clean":"Wait for trigger"}
{"time":"2026-05-18T09:17:59.12411457+05:30","level":"INFO","msg":"telegram: new signal received","raw_len":59,"clean":"BUY NIFTY 50 23050 PE ABOVE 28 TARGET 35-45+++ SL 21"}
{"time":"2026-05-18T09:17:59.124146164+05:30","level":"INFO","msg":"telegram: new signal received","raw_len":43,"clean":"SENSEX 74600 PE ABV 600 SL 500 TGT 650/700+"}
{"time":"2026-05-18T09:17:59.12417876+05:30","level":"INFO","msg":"telegram: new signal received","raw_len":46,"clean":"Buy Nifty 23350 pe 100 Target 110,130+ Sl 85"}
```

Daily logging path format:

```go
now.Format("02012006") + ".log"
```

Current processor constants:

```go
const (
    signalBatchWindow = 900 * time.Millisecond
    signalBatchMax    = 12
    signalWorkers     = 3
    signalTimeout     = 5 * time.Minute
)
```

Current quote monitor interval:

```go
const quoteMonitorInterval = 2 * time.Second
```

Current internal live quote event format:

```text
LIVE_QUOTE_UPDATE
event: PENDING_TRIGGER_HIT | TARGET_HIT
id: <trade id>
status: pending|active
symbol: <broker symbol>
exchange: NFO|BFO
index: NIFTY|SENSEX
strike: <strike>
option_type: CE|PE
ltp: <price>
entry_trigger: <price>
entry_upper: <price>
entry_price: <price>
stop_loss: <price>
targets: [..]
targets_hit: <n>
quantity: <units>
sl_order_id: <order id>
instruction: Use TRADING_MEMORY and this live quote to enter, trail, exit, or ignore according to the system rules.
```

Validation command:

```bash
go test ./... -count=1
```

# Suggested Next Actions

1. Review `cmd/bot/processor.go` and `cmd/bot/quote_monitor.go` for concurrency behavior and shutdown semantics.
2. Add tests for `quoteMonitor.eventForTrade`.
3. Run bot in `DRY_RUN=true` during market/simulated signal burst and inspect `logs/DDMMYYYY.log`.
4. Confirm batched prompt behavior: one LLM response should handle multiple independent entry signals in a newline batch.
5. Implement TradeManager actor as next major architecture step.
6. Move broker side effects into TradeManager or deterministic command layer.
7. Persist trade memory to disk and reconcile on startup.
8. Consider OpenAlgo websocket LTP subscriptions after polling behavior is validated.

# One Paragraph Super-Compressed Recovery Summary

Tele-Trader is a Go Telegram userbot in `/home/agspades/projects/tele-trader` that uses Claude Sonnet (`claude-sonnet-4-6`) plus OpenAlgo/Zerodha tools to parse noisy Indian options channel calls and execute/manage trades. The user wants reliable NIFTY/SENSEX signal execution from a fast Telegram channel. Work completed: prompt updated for ZERO TO HERO style messages, NIFTY/SENSEX exchange mapping, channel SL preference, no-chase rules, `cancel_order`, structured in-process `TradeMemory`, internal memory tools, daily JSON logs `logs/DDMMYYYY.log`, channel ID normalization, burst batching (`900ms`, max 12), worker pool (`3`), relaxed LLM locks, and polling quote monitor (`2s`) emitting `LIVE_QUOTE_UPDATE`. Tests pass with `go test ./... -count=1`. Real log showed old serial processing missed timely signals: many messages arrived at `09:17:59` but were processed over minutes. Current implementation is an improved intermediate design, not final: quote monitor still routes through LLM, trade memory is not persisted, concurrent LLM snapshots can be stale, and a true TradeManager actor should be next to own state and broker side effects deterministically.
