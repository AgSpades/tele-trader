package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/term"
)

// Handler wraps the gotd Telegram client and manages the userbot lifecycle.
type Handler struct {
	appID     int
	appHash   string
	phone     string
	channelID int64
	session   *FileSessionStorage
	// readyCh is closed once authentication completes successfully, signalling
	// to the caller that the session is valid and the bot's identity is confirmed.
	readyCh chan struct{}
}

// NewHandler creates a new Telegram Handler.
func NewHandler(appID int, appHash, phone string, channelID int64, session *FileSessionStorage) *Handler {
	return &Handler{
		appID:     appID,
		appHash:   appHash,
		phone:     phone,
		channelID: channelID,
		session:   session,
		readyCh:   make(chan struct{}),
	}
}

// ReadyCh returns a channel that is closed once the Telegram session is
// authenticated and client.Self() has succeeded. Use this to gate operations
// that require a live, verified Telegram session.
func (h *Handler) ReadyCh() <-chan struct{} {
	return h.readyCh
}

// stdinAuthenticator implements auth.UserAuthenticator, prompting the user
// interactively for OTP and 2FA password. The 2FA password is read with
// terminal echo disabled so it is never displayed on screen.
type stdinAuthenticator struct {
	phone string
}

// Phone returns the configured phone number without prompting.
func (a stdinAuthenticator) Phone(_ context.Context) (string, error) {
	return a.phone, nil
}

// Code prompts the user to enter the OTP received via Telegram/SMS.
func (a stdinAuthenticator) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	slog.Info("telegram: OTP required — enter the Telegram verification code:")
	fmt.Print("> ")
	var code string
	if _, err := fmt.Fscan(os.Stdin, &code); err != nil {
		return "", fmt.Errorf("reading OTP from stdin: %w", err)
	}
	return strings.TrimSpace(code), nil
}

// Password prompts for the Telegram 2FA cloud password.
// Input is hidden (no terminal echo) so the password is never visible.
func (a stdinAuthenticator) Password(_ context.Context) (string, error) {
	slog.Info("telegram: 2FA password required — enter your Telegram cloud password:")
	fmt.Print("> ")

	// term.ReadPassword disables echo, reads until Enter, then restores the terminal.
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println() // print newline after hidden input
	if err != nil {
		// Fallback to plain stdin read if not running in a TTY (e.g. piped input).
		slog.Warn("telegram: terminal not a TTY, falling back to plain stdin read")
		var pw string
		if _, scanErr := fmt.Fscan(os.Stdin, &pw); scanErr != nil {
			return "", fmt.Errorf("reading 2FA password: %w", scanErr)
		}
		return strings.TrimSpace(pw), nil
	}
	return string(raw), nil
}

// AcceptTermsOfService auto-accepts Telegram's Terms of Service.
// Required for new account sign-ups; existing accounts rarely trigger this.
func (a stdinAuthenticator) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	slog.Info("telegram: auto-accepting Terms of Service", "min_age", tos.MinAgeConfirm)
	return nil
}

// SignUp returns an error — this bot should only authenticate to existing accounts.
func (a stdinAuthenticator) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("telegram: sign-up is not supported; please register the account first")
}

// --- message cleaning helpers ---

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
// On first run it will interactively prompt for OTP and (if 2FA is enabled) the
// cloud password. Subsequent runs reuse the persisted session — no prompts needed.
// It blocks until ctx is cancelled.
//
// ReadyCh() is closed once authentication succeeds and client.Self() returns,
// allowing callers to gate on Telegram being fully ready.
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

		// Non-blocking send — drop if the downstream processor is full (rare).
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

		flow := auth.NewFlow(
			stdinAuthenticator{phone: h.phone},
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

		// Signal to waiters (e.g. healthcheck) that Telegram is ready.
		close(h.readyCh)

		// Block until ctx is cancelled (graceful shutdown).
		<-ctx.Done()
		return nil
	})
}
