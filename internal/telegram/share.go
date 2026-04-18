package telegram

import (
	"context"
	"errors"
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
			if errors.Is(err, errDuplicate) {
				b.replyDuplicate(msg.Chat.ID, result, contextText)
				return
			}
			b.captureErrorReply(msg.Chat.ID, err, "share capture failed", "url", urls[0])
			return
		}

		title, _ := result["title"].(string)
		artType, _ := result["artifact_type"].(string)
		artifactID, _ := result["artifact_id"].(string)
		connections := 0
		if c, ok := result["connections"].(float64); ok {
			connections = int(c)
		}

		suffix := ""
		if ps, _ := result["processing_status"].(string); ps == "pending" {
			suffix = " (processing pending)"
		}

		if contextText != "" {
			b.replyWithMapping(ctx, msg.Chat.ID, fmt.Sprintf(". Saved: \"%s\" (%s, %d connections) with context%s", title, artType, connections, suffix), artifactID)
		} else {
			b.replyWithMapping(ctx, msg.Chat.ID, fmt.Sprintf(". Saved: \"%s\" (%s, %d connections)%s", title, artType, connections, suffix), artifactID)
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
		// Strip trailing punctuation that's not part of URLs,
		// but preserve balanced parentheses (e.g. Wikipedia URLs).
		word = trimTrailingPunctuation(word)
		if (strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://")) && !seen[word] {
			urls = append(urls, word)
			seen[word] = true
		}
	}
	return urls
}

// trimTrailingPunctuation strips trailing punctuation from a URL-candidate word,
// preserving balanced parentheses that are part of the URL (e.g. Wikipedia links
// like https://en.wikipedia.org/wiki/Go_(programming_language)).
func trimTrailingPunctuation(word string) string {
	for len(word) > 0 {
		last := word[len(word)-1]
		switch last {
		case '.', ',', ';', ':', '!', '?', '"', '\'', '>':
			word = word[:len(word)-1]
		case ')':
			// Only strip ')' if it's unbalanced (more closing than opening)
			if strings.Count(word, "(") < strings.Count(word, ")") {
				word = word[:len(word)-1]
			} else {
				return word
			}
		case ']':
			if strings.Count(word, "[") < strings.Count(word, "]") {
				word = word[:len(word)-1]
			} else {
				return word
			}
		default:
			return word
		}
	}
	return word
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

// replyDuplicate sends a rich duplicate-detection reply per spec SC-TSC04.
// It extracts the title from the capture API 409 response when available and
// indicates whether new context was merged.
func (b *Bot) replyDuplicate(chatID int64, result map[string]interface{}, contextText string) {
	title, _ := result["title"].(string)
	if title == "" {
		title = "item"
	}
	if contextText != "" {
		b.reply(chatID, fmt.Sprintf(". Already saved: \"%s\" — updated with new context", title))
	} else {
		b.reply(chatID, fmt.Sprintf(". Already saved: \"%s\"", title))
	}
}
