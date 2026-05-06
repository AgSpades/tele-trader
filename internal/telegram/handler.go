package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"unicode"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// Handler wraps the gotd Telegram client and manages the userbot lifecycle.
type Handler struct {
	appID     int
	appHash   string
	phone     string
	channelID int64
	session   *FileSessionStorage
}

// NewHandler creates a new Telegram Handler.
func NewHandler(appID int, appHash, phone string, channelID int64, session *FileSessionStorage) *Handler {
	return &Handler{
		appID:     appID,
		appHash:   appHash,
		phone:     phone,
		channelID: channelID,
		session:   session,
	}
}

// emojiAndTimestampRe strips timestamps (e.g. "09:15 AM") from messages.
var emojiAndTimestampRe = regexp.MustCompile(`\b\d{1,2}:\d{2}(?:\s?[APap][Mm])?\b`)

// cleanMessage removes noise (timestamps, emoji, extraneous whitespace) from a Telegram message.
func cleanMessage(raw string) string {
	cleaned := emojiAndTimestampRe.ReplaceAllString(raw, "")

	var sb strings.Builder
	for _, r := range cleaned {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			sb.WriteRune(r)
		case r == ' ' || r == '\n' || r == '+' || r == '-' || r == '.' ||
			r == '/' || r == ':' || r == ',' || r == '%' || r == '🔥':
			sb.WriteRune(r)
		}
	}

	return strings.TrimSpace(strings.Join(strings.Fields(sb.String()), " "))
}

// Start connects the Telegram userbot and begins dispatching messages to msgCh.
// It blocks until ctx is cancelled.
func (h *Handler) Start(ctx context.Context, msgCh chan<- string) error {
	dispatcher := tg.NewUpdateDispatcher()

	// Listen for new channel messages.
	dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewChannelMessage) error {
		msg, ok := update.Message.(*tg.Message)
		if !ok || msg.Message == "" {
			return nil
		}

		// Filter by configured channel ID.
		peer, ok := msg.PeerID.(*tg.PeerChannel)
		if !ok || peer.ChannelID != h.channelID {
			return nil
		}

		clean := cleanMessage(msg.Message)
		if clean == "" {
			slog.Debug("telegram: message cleaned to empty, skipping")
			return nil
		}

		slog.Info("telegram: new signal received", "raw_len", len(msg.Message), "clean", clean)

		// Non-blocking send — drop if downstream processor is full (rare).
		select {
		case msgCh <- clean:
		default:
			slog.Warn("telegram: message channel full, signal dropped", "clean", clean)
		}
		return nil
	})

	client := telegram.NewClient(h.appID, h.appHash, telegram.Options{
		SessionStorage: h.session,
		UpdateHandler:  dispatcher,
	})

	return client.Run(ctx, func(ctx context.Context) error {
		slog.Info("telegram: client started, authenticating…")

		// codeReader prompts the user for the OTP code on first run.
		codeReader := auth.CodeAuthenticatorFunc(
			func(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
				slog.Info("telegram: OTP required — enter the Telegram verification code:")
				var code string
				if _, err := fmt.Scan(&code); err != nil {
					return "", fmt.Errorf("reading OTP: %w", err)
				}
				return strings.TrimSpace(code), nil
			},
		)

		flow := auth.NewFlow(
			auth.Constant(h.phone, "", codeReader),
			auth.SendCodeOptions{},
		)

		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return fmt.Errorf("telegram: auth: %w", err)
		}

		self, err := client.Self(ctx)
		if err != nil {
			return fmt.Errorf("telegram: get self: %w", err)
		}
		slog.Info("telegram: authenticated", "username", self.Username, "id", self.ID)
		slog.Info("telegram: listening for signals", "channel_id", h.channelID)

		// Block until ctx is cancelled (graceful shutdown).
		<-ctx.Done()
		return nil
	})
}
