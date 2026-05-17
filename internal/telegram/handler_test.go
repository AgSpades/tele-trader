package telegram

import (
	"strings"
	"testing"
)

// TestCleanMessage verifies that cleanMessage correctly strips timestamps, emoji,
// and non-trading characters while preserving the signal content that the LLM needs.
func TestCleanMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string // expected clean output
	}{
		// --- Entry signals ---
		{
			name: "plain entry signal",
			raw:  "SENSEX 77500 CE at 320",
			want: "SENSEX 77500 CE at 320",
		},
		{
			name: "entry signal with timestamp prefix",
			raw:  "09:15 AM NIFTY 26000 CE at 150",
			want: "NIFTY 26000 CE at 150",
		},
		{
			name: "entry signal with 24h timestamp",
			raw:  "09:15 NIFTY 26000 PE at 200",
			want: "NIFTY 26000 PE at 200",
		},
		{
			name: "entry signal with trailing emoji",
			raw:  "BANKNIFTY 55000 CE at 420 🚀🔔",
			want: "BANKNIFTY 55000 CE at 420",
		},
		{
			name: "entry signal with fire emoji (trailing signal)",
			raw:  "300🔥",
			want: "300🔥",
		},
		{
			name: "momentum update with fire emoji",
			raw:  "320🔥 trail SL",
			want: "320🔥 trail SL",
		},

		// --- Noisy real-world formats ---
		{
			name: "message with leading timestamp and emoji noise",
			raw:  "⏰ 10:30 AM — SENSEX 77200 PE buy at 180",
			want: "SENSEX 77200 PE buy at 180",
		},
		{
			name: "message with multiple emojis",
			raw:  "💰💰 NIFTY 25500 CE @ 95 🎯",
			want: "NIFTY 25500 CE 95",
		},
		{
			name: "book profits signal",
			raw:  "Book profits now ✅",
			want: "Book profits now",
		},
		{
			name: "SL hit message",
			raw:  "🔴 SL Hit — NIFTY 26000 CE",
			want: "SL Hit NIFTY 26000 CE",
		},
		{
			name: "multi-line signal",
			raw:  "NIFTY 26000 CE\nat 150\nSL: 120\nTarget: 200",
			want: "NIFTY 26000 CE at 150 SL: 120 Target: 200",
		},

		// --- Messages that should clean to empty ---
		{
			name: "emoji-only message",
			raw:  "🙏🙏🙏",
			want: "",
		},
		{
			name: "empty string",
			raw:  "",
			want: "",
		},
		{
			name: "only whitespace",
			raw:  "   \n\t  ",
			want: "",
		},

		// --- Preserve trading characters ---
		{
			name: "price with decimal",
			raw:  "NIFTY 26000 CE LTP 125.50",
			want: "NIFTY 26000 CE LTP 125.50",
		},
		{
			name: "percentage in message",
			raw:  "Trail SL up by 15%",
			want: "Trail SL up by 15%",
		},
		{
			name: "plus sign for OTM strikes",
			raw:  "OTM+2 NIFTY CE",
			want: "OTM+2 NIFTY CE",
		},
		{
			name: "slash in expiry notation",
			raw:  "NIFTY 30/12 26000 CE",
			want: "NIFTY 30/12 26000 CE",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := cleanMessage(tc.raw)
			if got != tc.want {
				t.Errorf("cleanMessage(%q)\n  got  %q\n  want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestCleanMessageTimestampVariants exercises the timestamp regex specifically.
func TestCleanMessageTimestampVariants(t *testing.T) {
	t.Parallel()

	timestamps := []string{
		"9:15 AM ",
		"09:15 AM ",
		"9:15AM ",
		"09:15 ",
		"15:30 ",
	}

	payload := "SENSEX 77500 CE at 320"
	for _, ts := range timestamps {
		ts := ts
		t.Run("prefix="+strings.TrimSpace(ts), func(t *testing.T) {
			t.Parallel()
			raw := ts + payload
			got := cleanMessage(raw)
			if got != payload {
				t.Errorf("cleanMessage(%q) = %q, want %q", raw, got, payload)
			}
		})
	}
}

// TestChannelFilter verifies the channel ID filtering logic directly.
func TestChannelFilter(t *testing.T) {
	t.Parallel()

	const wantChannelID int64 = -1001234567890
	// Extract numeric abs value used in PeerChannel (gotd stores positive IDs).
	const absID int64 = 1234567890

	tests := []struct {
		name      string
		channelID int64 // peer channel ID as seen in PeerChannel
		wantPass  bool
	}{
		{name: "matching channel passes", channelID: absID, wantPass: true},
		{name: "different channel is dropped", channelID: 9999999999, wantPass: false},
		{name: "zero id is dropped", channelID: 0, wantPass: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Negative channel IDs in Telegram API have prefix -100; gotd
			// PeerChannel.ChannelID is the positive numeric part.
			configuredAbs := normalizeChannelID(wantChannelID)
			got := tc.channelID == configuredAbs
			if got != tc.wantPass {
				t.Errorf("channelID=%d: filter=%v, want %v", tc.channelID, got, tc.wantPass)
			}
		})
	}
}
