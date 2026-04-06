# User Validation: 008 — Telegram Share & Chat Capture

> **Feature:** [specs/008-telegram-share-capture](.)
> **Status:** Not Started

---

## Acceptance Checklist

- [ ] Share URL with context from mobile share sheet captures both URL and context text
- [ ] Bare URL capture behavior is unchanged (backward compatible)
- [ ] Multiple URLs in a single message are each captured individually
- [ ] Duplicate URL share merges new context without re-processing
- [ ] Forwarded messages preserve original sender, source chat, and timestamp
- [ ] Privacy-restricted forwarded messages handled gracefully (no errors)
- [ ] Forwarded channel posts include channel attribution
- [ ] Forwarded message containing a URL is captured as forwarded artifact with source attribution
- [ ] Multiple forwarded messages from same source assemble into one conversation artifact
- [ ] Conversation artifact includes participant list and chronological ordering
- [ ] Single forwarded message is captured as individual artifact (not conversation)
- [ ] `/done` command flushes active assembly immediately
- [ ] Assembly buffer overflow produces correctly split conversation artifacts
- [ ] URLs within a forwarded conversation are NOT separately captured
- [ ] Media groups (shared photos/videos) assemble into single artifact
- [ ] Media group captions are preserved and concatenated
- [ ] Assembled conversations are searchable by participant name and topic
- [ ] Unauthorized chats cannot trigger assembly or capture
- [ ] Assembly buffers are isolated between different users/chats
- [ ] Configuration parameters are tunable via smackerel.yaml
- [ ] Invalid configuration values fail explicitly at startup
- [ ] All confirmation messages use text markers (no emoji)
- [ ] Existing bot functionality (commands, voice, text) is unaffected
