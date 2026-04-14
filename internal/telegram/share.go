package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/stringutil"
)

// maxShareTextLen is the maximum accepted length for share/forward text input.
const maxShareTextLen = 4096

// handleShareCapture handles messages containing URLs with optional context text.
// It supports share-sheet payloads that include both a URL and descriptive text
// from the sending app.
func (b *Bot) handleShareCapture(ctx context.Context, msg *tgbotapi.Message, text string) {
	if len(text) > maxShareTextLen {
		text = stringutil.TruncateUTF8(text, maxShareTextLen)
	}
	urls := extractAllURLs(text)
	if len(urls) == 0 {
		b.reply(msg.Chat.ID, "? Couldn't find a URL in your message")
		return
	}

	contextText := extractContext(text, urls)

	if len(urls) == 1 {
		body := map[string]string{"url": urls[0]}
		if contextText != "" {
			body["context"] = contextText
		}

		result, err := b.callCapture(ctx, body)
		if err != nil {
			b.captureErrorReply(msg.Chat.ID, err, "share capture failed", "url", urls[0])
			return
		}

		title, _ := result["title"].(string)
		artType, _ := result["artifact_type"].(string)
		connections := 0
		if c, ok := result["connections"].(float64); ok {
			connections = int(c)
		}

		suffix := ""
		if ps, _ := result["processing_status"].(string); ps == "pending" {
			suffix = " (processing pending)"
		}

		if contextText != "" {
			b.reply(msg.Chat.ID, fmt.Sprintf(". Saved: \"%s\" (%s, %d connections) with context%s", title, artType, connections, suffix))
		} else {
			b.reply(msg.Chat.ID, fmt.Sprintf(". Saved: \"%s\" (%s, %d connections)%s", title, artType, connections, suffix))
		}
		return
	}

	// Multiple URLs — capture each individually
	saved := 0
	for _, u := range urls {
		body := map[string]string{"url": u}
		if contextText != "" {
			body["context"] = contextText
		}
		if _, err := b.callCapture(ctx, body); err != nil {
			slog.Error("share multi-url capture failed", "error", err, "url", u)
			continue
		}
		saved++
	}

	if saved == 0 {
		b.reply(msg.Chat.ID, "? Failed to save URLs. Try again in a moment.")
	} else {
		b.reply(msg.Chat.ID, fmt.Sprintf(". Saved %d of %d URLs", saved, len(urls)))
	}
}

// extractAllURLs returns all http:// and https:// URLs found in text.
func extractAllURLs(text string) []string {
	var urls []string
	seen := make(map[string]bool)
	for _, word := range strings.Fields(text) {
		// Strip leading brackets/parens that wrap URLs
		word = strings.TrimLeft(word, "(<[")
		// Strip trailing punctuation that's not part of URLs
		word = strings.TrimRight(word, ".,;:!?\"')>]")
		if (strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://")) && !seen[word] {
			urls = append(urls, word)
			seen[word] = true
		}
	}
	return urls
}

// extractContext removes all URLs from text and returns the remaining context.
// URLs are removed longest-first so that a shorter URL that is a prefix of a
// longer one does not corrupt the longer URL during replacement.
func extractContext(text string, urls []string) string {
	// Sort URLs by descending length so longer URLs are removed before any
	// shorter URL that happens to be a prefix of them.
	sorted := make([]string, len(urls))
	copy(sorted, urls)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i]) > len(sorted[j])
	})

	result := text
	for _, u := range sorted {
		result = strings.ReplaceAll(result, u, "")
	}
	// Collapse multiple whitespace
	fields := strings.Fields(result)
	return strings.Join(fields, " ")
}
