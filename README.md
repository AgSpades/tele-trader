# Tele-Trader

An AI-powered Telegram userbot that autonomously reads trading signals from a specified Telegram channel and executes trades in the Indian Derivatives Market (NFO/BFO) using [OpenAlgo](https://openalgo.in/).

> **Status:** Under Development

## Overview

Tele-Trader uses **Anthropic's Claude 4.6 Sonnet** as an autonomous agent. Instead of relying on brittle regex parsing to read complex, informal Telegram signals, the bot streams live messages directly to the LLM. 

The LLM is equipped with a suite of OpenAlgo execution tools, allowing it to:
1. Parse informal trade ideas (e.g., `"SENSEX 77500 CE at 320"`) and momentum trailing updates (e.g., `"350🔥"`).
2. Search for the exact broker symbol.
3. Validate current market prices (LTP) against the signal price.
4. Execute Market Orders and automatically place protective Stop-Loss Limit orders.
5. Autonomously fetch the order book and modify pending Stop-Loss orders to trail prices.

## Features

- **True AI Parsing:** Understands human nuance, ignoring noise like "Good morning", polls, and general commentary.
- **Strict Risk Management:** Hardcoded trade sizing (lots per trade) and initial stop-loss percentages configured via environment variables.
- **Dynamic Trailing Stop-Loss:** Reacts to momentum updates (e.g. `"135🔥"`) by finding the active SL order and modifying its trigger and limit prices.
- **Broker Agnostic (via OpenAlgo):** Designed primarily around Zerodha's specific NFO/BFO restrictions (e.g., no SL-M orders allowed, enforcing SL-Limit usage), but easily portable.
- **Interactive Authentication:** Handles Telegram OTP and 2FA dynamically on a fresh login without timing out the startup sequence.
- **Dry Run Mode:** Safe testing using OpenAlgo's Analyzer without placing real capital at risk.

## Configuration

The bot is configured via environment variables (or a `.env` file in the root directory).

| Variable | Description | Default |
|----------|-------------|---------|
| `TELEGRAM_APP_ID` | Your Telegram API ID | *Required* |
| `TELEGRAM_APP_API_HASH`| Your Telegram API Hash | *Required* |
| `TELEGRAM_PHONE` | Your Telegram Phone Number (with country code) | *Required* |
| `TELEGRAM_CHANNEL_ID` | The numeric ID of the channel to listen to (e.g. `123456789`) | *Required* |
| `ANTHROPIC_API_KEY` | Your Anthropic Claude API Key | *Required* |
| `OPENALGO_API_KEY` | OpenAlgo API Key | *Required* |
| `OPENALGO_URL` | OpenAlgo Local/Remote URL | `http://127.0.0.1:5000` |
| `SESSION_FILE_PATH` | Path to store the Telegram session | `session.json` |
| `DRY_RUN` | If `true`, routes orders to OpenAlgo Analyzer | `true` |
| `TRADE_LOT_SIZE` | Fixed number of lots to execute per entry | `1` |
| `OPENING_SL_PERCENT`| Initial stop-loss percentage distance from entry | `15.0` |

## Getting Started

1. Ensure you have Go 1.21+ installed.
2. Clone the repository and copy your `.env` file.
3. Run the bot:

```bash
go run ./cmd/bot/...
```
*(Or use `make run` if configured)*

On the very first run, the bot will pause during startup and ask for your Telegram OTP (and 2FA password if enabled) in the console. Once authenticated, a `session.json` file is generated, and subsequent startups will be completely headless.

## Architecture Highlights

- **`internal/telegram`**: Connects via `gotd/td` as a userbot to listen to restricted or private channels where standard bots aren't allowed.
- **`internal/llm`**: The core "brain" utilizing Anthropic's tool-calling capabilities. Governed by a strict system prompt (SOP) that outlines exact Zerodha execution rules.
- **`internal/broker`**: A wrapper around the `openalgo-go` SDK exposing vital broker methods to the LLM (`place_order`, `get_quote`, `search_instruments`, `get_order_book`, etc.).

## Disclaimer

This software is for educational purposes. Automated trading carries significant financial risk. Always test extensively in `DRY_RUN=true` mode before connecting to a live broker account.
