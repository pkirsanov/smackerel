# Smackerel

> *"What I like best is just doing nothing... and having a little smackerel of something." — Winnie-the-Pooh*

A passive intelligence layer across your entire digital life. It observes, captures, connects, and synthesizes information so you don't have to organize anything yourself.

## What It Does

- **Observes** everything — email, videos, maps, calendar, browsing, notes, purchases
- **Captures** anything via zero-friction input from any device
- **Connects** across domains — cross-links, detects themes, builds a living knowledge graph
- **Searches** by meaning, not keywords — "that pricing video" just works
- **Synthesizes** patterns, proposes ideas, identifies blind spots
- **Evolves** — promotes hot topics, archives cold ones, tracks expertise growth
- **Surfaces** the right information at the right time
- **Runs locally** — you own your data, always

## Docs

- [Design Document](docs/smackerel.md)
- [Development Guide](docs/Development.md)
- [Testing Guide](docs/Testing.md)
- [Docker Best Practices](docs/Docker_Best_Practices.md)

## Quick Start

**Prerequisites:** Docker and Docker Compose.

```bash
# 1. Clone and enter the repo
git clone <repo-url> && cd smackerel

# 2. Edit config (LLM provider, auth token, etc.)
#    See "Configuration" section below for details
nano config/smackerel.yaml

# 3. Generate runtime env files from config
./smackerel.sh config generate

# 4. Build Docker images
./smackerel.sh build

# 5. Start the stack
./smackerel.sh up

# 6. Verify — all services should be "up"
curl http://127.0.0.1:40001/api/health
```

The stack runs 4 containers under the `smackerel` Compose project:

| Service | Description | Default Host Port |
|---------|-------------|-------------------|
| `smackerel-core` | Go API + Telegram bot + scheduler | `40001` |
| `smackerel-ml` | Python ML sidecar (LLM, embeddings, transcription) | `40002` |
| `postgres` | PostgreSQL 16 + pgvector | `42001` |
| `nats` | NATS JetStream message bus | `42002` |

## Configuration

All configuration lives in **`config/smackerel.yaml`**. After editing, always run:

```bash
./smackerel.sh config generate
./smackerel.sh down && ./smackerel.sh up
```

### Authentication

The API uses Bearer token authentication. Set a strong token for any non-local use:

```yaml
runtime:
  auth_token: your-secret-token-here   # default: development-change-me
```

Use it in API calls:

```
Authorization: Bearer your-secret-token-here
```

### LLM Provider

The ML sidecar uses [litellm](https://docs.litellm.ai/) to route to any LLM. Configure in `config/smackerel.yaml`:

**Anthropic (recommended):**
```yaml
llm:
  provider: anthropic
  model: claude-sonnet-4-20250514        # or claude-3-haiku for lower cost
  api_key: sk-ant-api03-...
```

**OpenAI:**
```yaml
llm:
  provider: openai
  model: gpt-4o-mini
  api_key: sk-proj-...
```

**Ollama (local, free, no API key):**
```yaml
llm:
  provider: ollama
  model: llama3.2
  api_key: ""
  ollama_url: http://ollama:11434

infrastructure:
  ollama:
    enabled: true                       # starts the Ollama container
```

The LLM is used for: content processing (entity/topic extraction), search re-ranking, and digest generation. Embeddings always run locally via `all-MiniLM-L6-v2` in the ML sidecar regardless of LLM provider.

### Telegram Bot

1. Message [@BotFather](https://t.me/BotFather) on Telegram → `/newbot` → copy the token
2. Message [@userinfobot](https://t.me/userinfobot) → copy your numeric chat ID
3. Configure:

```yaml
telegram:
  bot_token: "7123456789:AAH..."
  chat_ids: "123456789"              # comma-separated for multiple users
```

4. Regenerate and restart. The bot accepts:
   - **Any URL** → captures and processes the article/video/product
   - **Plain text** → saved as idea/note
   - **Voice note** → transcribed via Whisper, then processed
   - `/find <query>` → semantic search (top 3 results)
   - `/digest` → today's daily digest
   - `/status` → system stats

Messages from chat IDs not in the allowlist are silently ignored.

### Daily Digest

The digest cron fires at the configured time (default 7:00 AM), assembles action items + overnight artifacts + hot topics, generates a summary via LLM, and delivers via the API and Telegram (if configured).

```yaml
runtime:
  digest_cron: "0 7 * * *"            # cron expression (default: 7 AM daily)
```

### Connectors (Passive Ingestion)

Connectors run on cron schedules and sync data incrementally using cursors stored in the `sync_state` table. Each connector requires opt-in configuration.

#### Gmail (IMAP)

Ingests email via IMAP with OAuth2 XOAUTH2 authentication. A single Google OAuth consent screen covers Gmail + Calendar + YouTube.

1. Create OAuth2 credentials in [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Enable the Gmail API
3. Required OAuth scope: `https://mail.google.com/` (read-only access)
4. Configure the connector with OAuth2 credentials and priority rules:
   - **Priority senders** → emails from these addresses get full LLM processing
   - **Skip labels** → emails with these labels are stored as metadata only
   - **Priority labels** → emails with these labels (Starred, Important) get full processing

#### Google Calendar (CalDAV)

Syncs calendar events via CalDAV protocol with OAuth2 authentication.

1. Use the same Google OAuth2 credentials as Gmail
2. Required OAuth scope: `https://www.googleapis.com/auth/calendar.readonly`
3. Events are processed for: attendee linking to People entities, pre-meeting context assembly (related artifacts for each attendee), and temporal linking

#### YouTube

Syncs liked videos, watch later, and playlist content via YouTube Data API v3.

1. Use the same Google OAuth2 credentials
2. Required OAuth scope: `https://www.googleapis.com/auth/youtube.readonly`
3. Processing tiers based on engagement:
   - **Liked videos** → full processing (transcript + summary + entities)
   - **Playlist videos** → full processing
   - **Watch later** → standard processing
   - **History** → light processing
4. Transcripts are fetched via `youtube-transcript-api` in the ML sidecar; falls back to Whisper for audio-only

#### RSS / Atom Feeds

Subscribes to RSS and Atom feeds. Configure feed URLs through the settings UI or sync_state table. Each feed is polled on a schedule and new items are processed through the standard pipeline.

#### Bookmarks Import

Import bookmarks from any browser:
- **Chrome**: Export as JSON from `chrome://bookmarks` → upload via the API
- **Firefox/Safari/other**: Export as Netscape HTML format → upload via the API
- Folder names are mapped to topics automatically
- Duplicates detected by URL content hash

#### Google Maps Timeline

Imports location history from Google Takeout. Requires explicit privacy consent.

1. Export location history from [Google Takeout](https://takeout.google.com/) (JSON format)
2. Upload the JSON file via the API
3. Activities are classified: walk, cycle, drive, transit, hike, run
4. Routes stored as GeoJSON; qualifying walks/hikes (2+ km) are tagged as trails
5. Privacy consent must be granted per-source before import is accepted

#### Browser History

Imports browsing history from Chrome's SQLite database. Requires explicit privacy consent.

1. Privacy consent must be granted before sync is enabled
2. Processing tiers based on dwell time:
   - **5+ minutes** → full processing (fetch content, extract summary)
   - **2-5 minutes** → standard processing
   - **30s-2min** → light processing
   - **Under 30s** → metadata only
3. Social media domains (Twitter, Reddit, etc.) are aggregated rather than individually processed
4. Configurable skip-list for domains to never process (e.g., localhost, internal tools)

## API Usage

All API endpoints (except `/api/health`) require the Bearer token.

### Capture Content

```bash
# Save a text note
curl -X POST http://127.0.0.1:40001/api/capture \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{"text": "Organize team by customer segment"}'

# Save a URL (auto-detects type: article, YouTube, product, recipe)
curl -X POST http://127.0.0.1:40001/api/capture \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/article"}'

# Save with context (improves LLM processing)
curl -X POST http://127.0.0.1:40001/api/capture \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{"text": "Annual contracts reduce churn", "context": "Sarah recommended"}'

# Save a voice note URL
curl -X POST http://127.0.0.1:40001/api/capture \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{"voice_url": "https://example.com/voice.ogg"}'
```

**Response:** `{"artifact_id": "...", "title": "...", "artifact_type": "article", "summary": "...", "connections": 3}`

**Error codes:** `400` invalid input, `401` unauthorized, `409` duplicate detected, `503` ML sidecar unavailable.

### Search

```bash
curl -X POST http://127.0.0.1:40001/api/search \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{"query": "that pricing video", "limit": 5}'

# With filters
curl -X POST http://127.0.0.1:40001/api/search \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{"query": "recommendations", "filters": {"person": "Sarah", "type": "video"}}'
```

Search pipeline: query → embed → pgvector cosine similarity (top 30) → metadata filters → knowledge graph expansion → LLM re-ranking → top results with relevance explanations.

### Daily Digest

```bash
# Get today's digest
curl -H "Authorization: Bearer your-token" \
  http://127.0.0.1:40001/api/digest

# Get a specific date
curl -H "Authorization: Bearer your-token" \
  "http://127.0.0.1:40001/api/digest?date=2026-04-05"
```

### Health Check

```bash
curl http://127.0.0.1:40001/api/health
```

Returns status of all services: api, postgres, nats, ml_sidecar, telegram_bot, ollama.

## Web UI

Open **http://127.0.0.1:40001** in a browser. Pages:

- **/** — Search with live HTMX-powered results
- **/topics** — Topic lifecycle view (emerging → active → hot → cooling → dormant)
- **/settings** — Connector status and configuration
- **/artifact?id=...** — Artifact detail with summary, key ideas, entities, connections

Dark/light theme follows OS preference. Monochrome design — no accent colors, no emoji.

## Runtime Standards

Smackerel now has a foundation runtime scaffold with a repo CLI, YAML-backed config generation, a Go core, a Python ML sidecar, and Docker Compose orchestration. The runtime still only covers the current scaffold scope, but the operational surface is now standardized:

- Docker-only runtime and test execution
- One repo CLI for build, test, config generation, stack lifecycle, logs, and cleanup: `./smackerel.sh`
- A single configuration source of truth in `config/smackerel.yaml` with generated runtime env artifacts
- A workspace-unique Docker host-forwarding block in `40000-49999` to avoid collisions with the other repos in this workspace
- Persistent development state separated from disposable test and validation state
- Smart cleanup and build-freshness verification instead of destructive default cleanup
- Live-stack integration and E2E requirements with isolated test environments

Current runtime entrypoints:

- `./smackerel.sh config generate`
- `./smackerel.sh build`
- `./smackerel.sh check`
- `./smackerel.sh lint`
- `./smackerel.sh format`
- `./smackerel.sh test unit`
- `./smackerel.sh test integration`
- `./smackerel.sh test e2e`
- `./smackerel.sh test stress`
- `./smackerel.sh up`
- `./smackerel.sh down`
- `./smackerel.sh status`
- `./smackerel.sh logs`
- `./smackerel.sh clean smart|full|status|measure`
