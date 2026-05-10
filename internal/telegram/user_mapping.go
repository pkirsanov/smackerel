// Spec 044 Scope 03 — Telegram bridge per-user identity (chat_id → user_id).
//
// The Telegram bot is a multi-user channel: every Telegram message
// arrives with a chat_id, but until spec 044 there was no mapping
// from chat_id to a Smackerel user_id. The bot called the internal
// API with a single shared bearer token, so every captured artifact
// was attributed to "no user" / "shared session" — closing
// MIT-027-TRACE-001's last open segment requires a deterministic
// chat_id → user_id mapping plus a production-mode rejection rule.
//
// This file adds:
//
//  1. Config.UserMapping (chat_id → user_id) — populated from
//     TELEGRAM_USER_MAPPING env var (config/smackerel.yaml SST).
//  2. Bot.Environment — production | development | test.
//  3. Bot.resolveActorUserID(chatID) — the entry-point lookup.
//     In production, an unmapped chat MUST be refused (the bot
//     drops the message with a slog.Warn). In dev/test, an empty
//     string is acceptable so the existing dev workflow is not
//     disrupted.
//
// Wiring (cmd/core/wiring.go) MUST pass cfg.Environment +
// cfg.TelegramUserMapping into telegram.NewBot.
package telegram

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// ErrNoUserMappingForChat is returned by Bot.resolveActorUserID when
// a Telegram chat_id is not present in the production user-mapping
// config. The message-handling layer drops the message; callers
// MUST NOT silently fall back to a synthetic actor.
var ErrNoUserMappingForChat = errors.New("telegram: chat has no production user mapping (TELEGRAM_USER_MAPPING)")

// ParseUserMapping converts a TELEGRAM_USER_MAPPING env value of the
// form "12345:alice,67890:bob" into a chat_id → user_id map.
// Whitespace around tokens is tolerated; empty input returns nil.
//
// Format intentionally mirrors TELEGRAM_CHAT_IDS (comma-separated
// pairs) so operators have one mental model for both knobs. Each pair
// is "<int64 chat_id>:<user_id>"; both halves are required and
// non-empty. Returns a wrapped error naming the offending pair.
func ParseUserMapping(raw string) (map[int64]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	out := make(map[int64]string)
	for idx, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			return nil, fmt.Errorf("telegram: TELEGRAM_USER_MAPPING entry %d is empty (format: chat_id:user_id, comma-separated)", idx+1)
		}
		colon := strings.IndexByte(pair, ':')
		if colon <= 0 || colon == len(pair)-1 {
			return nil, fmt.Errorf("telegram: TELEGRAM_USER_MAPPING entry %d %q is malformed (expected chat_id:user_id)", idx+1, pair)
		}
		chatRaw := strings.TrimSpace(pair[:colon])
		userID := strings.TrimSpace(pair[colon+1:])
		if userID == "" {
			return nil, fmt.Errorf("telegram: TELEGRAM_USER_MAPPING entry %d has empty user_id", idx+1)
		}
		chatID, err := strconv.ParseInt(chatRaw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("telegram: TELEGRAM_USER_MAPPING entry %d chat_id %q is not int64: %w", idx+1, chatRaw, err)
		}
		if _, dup := out[chatID]; dup {
			return nil, fmt.Errorf("telegram: TELEGRAM_USER_MAPPING entry %d duplicates chat_id %d", idx+1, chatID)
		}
		out[chatID] = userID
	}
	return out, nil
}

// resolveActorUserID looks up the canonical Smackerel user_id for a
// Telegram chat_id, applying the spec 044 production rejection rule.
//
// Returns:
//   - production + mapped chat → user_id, nil
//   - production + UN-mapped chat → "", ErrNoUserMappingForChat
//   - dev/test (any environment != "production") → mapping[chatID]
//     (which may be "") and nil. The dev workflow keeps working.
//
// Callers in production MUST treat a non-nil error as a hard refusal
// to process the message (no API call, no capture, no annotation).
func (b *Bot) resolveActorUserID(chatID int64) (string, error) {
	if b == nil {
		return "", ErrNoUserMappingForChat
	}
	if b.userMapping != nil {
		if user, ok := b.userMapping[chatID]; ok && user != "" {
			return user, nil
		}
	}
	if strings.EqualFold(b.environment, "production") {
		slog.Warn("telegram message refused — production chat has no TELEGRAM_USER_MAPPING entry",
			"chat_id", chatID,
			"environment", b.environment)
		return "", ErrNoUserMappingForChat
	}
	// Dev/test: allow the message through with an empty actor; the
	// bearer-auth dev/test path attaches a SessionSourceSharedToken
	// session whose UserID is "". Existing dev annotation/capture
	// flows already tolerate that.
	return "", nil
}
