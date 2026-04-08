<p align="center">
  <img src="assets/icons/logo-mark.svg" width="96" height="96" alt="Smackerel">
</p>

<h1 align="center">Smackerel</h1>

<p align="center">
  <em>"What I like best is just doing nothing... and having a little smackerel of something." — Winnie-the-Pooh</em>
</p>

<p align="center">A passive intelligence layer across your entire digital life.<br>It observes, captures, connects, and synthesizes information so you don't have to organize anything yourself.</p>

---

## What It Does

<img src="assets/icons/feature-observe.svg" width="20" height="20" alt="">&ensp;**Observes** everything — email, videos, maps, calendar, browsing, notes, purchases

<img src="assets/icons/feature-capture.svg" width="20" height="20" alt="">&ensp;**Captures** anything via zero-friction input from any device

<img src="assets/icons/feature-connect.svg" width="20" height="20" alt="">&ensp;**Connects** across domains — cross-links, detects themes, builds a living knowledge graph

<img src="assets/icons/feature-search.svg" width="20" height="20" alt="">&ensp;**Searches** by meaning, not keywords — "that pricing video" just works

<img src="assets/icons/feature-synthesize.svg" width="20" height="20" alt="">&ensp;**Synthesizes** patterns, proposes ideas, identifies blind spots

<img src="assets/icons/feature-evolve.svg" width="20" height="20" alt="">&ensp;**Evolves** — promotes hot topics, archives cold ones, tracks expertise growth

<img src="assets/icons/feature-surface.svg" width="20" height="20" alt="">&ensp;**Surfaces** the right information at the right time

<img src="assets/icons/feature-local.svg" width="20" height="20" alt="">&ensp;**Runs locally** — you own your data, always

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

The API uses Bearer token authentication. Generate a secure token (minimum 16 characters):

```bash
openssl rand -hex 24
```

Set it in config:

```yaml
runtime:
  auth_token: your-generated-token-here
```

Known placeholder values like `development-change-me` are rejected at startup. Use it in API calls:

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
   - **URL + context text** → share-sheet payloads preserve the description alongside the URL
   - **Plain text** → saved as idea/note
   - **Voice note** → transcribed via Whisper, then processed
   - **Forwarded messages** → captured with original sender, source chat, and timestamp metadata
   - **Forwarded conversation** → multiple forwarded messages from the same source are assembled into a single conversation artifact with participant extraction and timeline
   - **Media groups** → photos/videos shared together are assembled into a single multi-attachment artifact
   - `/find <query>` → semantic search (top 3 results)
   - `/digest` → today's daily digest
   - `/done` → finalize conversation assembly (flush all open buffers)
   - `/status` → system stats
   - `/recent` → last 5 captured artifacts

Messages from chat IDs not in the allowlist are silently ignored.

### Daily Digest

The digest cron fires at the configured time (default 7:00 AM), assembles action items + overnight artifacts + hot topics, generates a summary via LLM, and delivers via the API and Telegram (if configured).

```yaml
runtime:
  digest_cron: "0 7 * * *"            # cron expression (default: 7 AM daily)
```

### Connectors (Passive Ingestion)

Connectors run on 5-minute sync cycles via the supervisor and sync data incrementally using cursors stored in the `sync_state` table.

#### Google OAuth Setup (Gmail + Calendar + YouTube)

A single Google OAuth2 consent screen covers all three Google connectors. Set this up once:

1. Go to [Google Cloud Console](https://console.cloud.google.com/) → Create a project (or use existing)
2. Enable these APIs:
   - Gmail API
   - Google Calendar API
   - YouTube Data API v3
3. Go to **APIs & Services → Credentials → Create Credentials → OAuth 2.0 Client ID**
   - Application type: **Web application**
   - Authorized redirect URI: `http://127.0.0.1:40001/auth/google/callback`
4. Copy the Client ID and Client Secret
5. Configure in `config/smackerel.yaml`:

```yaml
oauth:
  google:
    client_id: "123456789-xxxxx.apps.googleusercontent.com"
    client_secret: "GOCSPX-xxxxxxxxxxxxxxxx"
    redirect_url: "http://127.0.0.1:40001/auth/google/callback"
```

6. Regenerate config and restart: `./smackerel.sh config generate && ./smackerel.sh down && ./smackerel.sh up`
7. Open `http://127.0.0.1:40001/auth/google/start` in your browser
8. Grant access to Gmail, Calendar, and YouTube
9. On successful callback, all three connectors start automatically

Tokens are stored in PostgreSQL with automatic refresh. Check connection status at `http://127.0.0.1:40001/auth/status`.

#### Gmail

Fetches email via the Gmail REST API using the OAuth2 token from above.

- Messages fetched from INBOX with incremental cursor
- Headers extracted: From, To, Subject, Date, Message-ID, In-Reply-To
- Body extracted: prefers text/plain, falls back to text/html
- Labels preserved for tier assignment
- Processing tiers:
  - **Priority senders** → full LLM processing
  - **Priority labels** (Starred, Important) → full processing
  - **Skip labels** (Promotions, Social) → metadata only
  - **Skip domains** (newsletters, noreply) → skipped entirely
  - **Default** → standard processing
- Action items extracted from email body (deadlines, todos, requests)

#### Google Calendar

Fetches events via Google Calendar API v3 using the same OAuth2 token.

- Supports time-based and syncToken cursors for incremental sync
- Extracts full event metadata: summary, description, location, organizer, attendees
- Handles all-day events and recurring events
- Processing tiers:
  - **Events with attendees** → full processing (pre-meeting context assembly)
  - **Solo events** → standard processing
  - **Recurring events** → light processing
  - **Cancelled events** → skipped
- Attendees linked to People entities in the knowledge graph

#### YouTube

Fetches videos via YouTube Data API v3 using the same OAuth2 token.

- Sources: Liked videos, Watch Later, custom playlists
- Deduplicates across sources (same video in liked + playlist)
- Processing tiers based on engagement:
  - **Liked videos** → full processing (transcript + summary + entities)
  - **Playlist videos** → full processing (tagged with playlist name)
  - **Watch Later** → standard processing
  - **Default** → light processing
- Transcripts fetched via `youtube-transcript-api`; falls back to Whisper

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

#### Google Keep

Imports notes from Google Keep via Takeout export or live gkeepapi bridge.

**Takeout mode (default):**
1. Export Keep data from [Google Takeout](https://takeout.google.com/) (JSON format)
2. Configure the import directory in `config/smackerel.yaml`:

```yaml
connectors:
  google-keep:
    enabled: true
    sync_mode: takeout
    import_dir: /path/to/takeout/Keep
```

3. Notes are parsed with full metadata: titles, labels, checklists, timestamps, attachments
4. Labels are automatically mapped to knowledge graph topics via 4-stage cascade: exact match → abbreviation → fuzzy (trigram similarity) → create new
5. Processing tiers: pinned → full, labeled → full, images → full, recent → standard, archived → light, trashed → skip
6. Image attachments can be OCR'd via Tesseract (primary) with Ollama vision fallback

**gkeepapi mode (optional):**
- Live sync via the unofficial gkeepapi Python library through the ML sidecar
- Requires Google app password and explicit opt-in acknowledgment
- Session caching for efficient re-authentication

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

Search pipeline: query → **temporal intent parsing** ("from last week" → auto-filter) → embed → pgvector cosine similarity (top 30) → metadata filters → **knowledge graph expansion** (discovers connected artifacts via edges) → **LLM re-ranking** (context-aware relevance ordering) → top results with relevance explanations.

Temporal expressions are automatically detected: "yesterday", "last week", "this month", "recently", etc. The temporal phrase is removed from the query and converted to date filters.

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

### Recent Items

```bash
curl -H "Authorization: Bearer your-token" \
  http://127.0.0.1:40001/api/recent
```

Returns the 5 most recently captured artifacts. Accepts optional `?limit=N` parameter (max 50).

### Export / Backup

```bash
# Export all artifacts as JSONL
curl -H "Authorization: Bearer your-token" \
  http://127.0.0.1:40001/api/export

# Paginated export (10K artifacts per page)
curl -H "Authorization: Bearer your-token" \
  "http://127.0.0.1:40001/api/export?limit=1000"

# Next page using cursor from X-Next-Cursor header
curl -H "Authorization: Bearer your-token" \
  "http://127.0.0.1:40001/api/export?cursor=2026-04-01T00:00:00Z&limit=1000"
```

Returns JSONL (one JSON object per line) with `Content-Disposition: attachment`. The response includes an `X-Next-Cursor` header for pagination. Maximum 10,000 artifacts per request.

### Artifact Detail

```bash
curl -H "Authorization: Bearer your-token" \
  http://127.0.0.1:40001/api/artifact/01HXYZ...
```

### OAuth Status

```bash
# Check which providers have valid tokens
curl http://127.0.0.1:40001/auth/status
```

## Intelligence Engine

Smackerel runs background intelligence jobs on a schedule:

| Job | Schedule | What it does |
|-----|----------|-------------|
| **Topic momentum** | Hourly | Updates topic lifecycle states (emerging → active → hot → cooling → dormant) based on capture frequency and decay |
| **Synthesis** | Daily at 2 AM | Detects cross-domain clusters (3+ artifacts sharing topics), identifies through-lines, contradictions, and patterns |
| **Overdue commitments** | Daily at 2 AM | Scans action items with passed deadlines, creates alerts |
| **Resurfacing** | Daily at 8 AM | Selects high-value dormant artifacts + serendipity picks, delivers via Telegram |
| **Daily digest** | Configurable cron | Assembles action items + hot topics + overnight artifacts → LLM summary → Telegram delivery |

All jobs have timeouts, nil-guards, and graceful failure handling. Digest delivery retries on the next cycle if the ML sidecar was slow.

## Security

| Protection | Implementation |
|-----------|----------------|
| **API authentication** | Bearer token (min 16 chars, placeholder values rejected at startup) |
| **NATS authentication** | Token auth enforced on all NATS connections (Go + Python + server) |
| **SSRF protection** | URL validation blocks private IPs, loopback, metadata endpoints, non-HTTP schemes. Redirect chains re-validated per hop. |
| **SQL injection** | All queries parameterized with `$N` placeholders, ILIKE metacharacters escaped |
| **XSS prevention** | Go `html/template` auto-escaping + `safeURL` blocks `javascript:`/`data:` schemes |
| **CSP header** | `default-src 'self'; script-src 'self' https://unpkg.com; style-src 'self' 'unsafe-inline'` |
| **Rate limiting** | 100 concurrent API requests (Chi middleware) |
| **Dedup integrity** | Unique partial index on `content_hash` + belt-and-suspenders check |
| **OAuth CSRF** | Crypto-random state tokens with 10-minute TTL and 100-entry cap |
| **Token storage** | OAuth tokens in PostgreSQL with automatic refresh on expiry |
| **Config validation** | Fail-fast on missing vars, validates PORT (1-65535), DIGEST_CRON (5-field), auth token strength |
| **Constant-time auth** | `subtle.ConstantTimeCompare` for token verification |
| **Body size limits** | 1MB API request bodies, 5MB RSS feeds, 10MB OCR images, 10MB article fetch |
| **Resource limits** | Docker memory limits: postgres 512M, nats 256M, core 256M, ml 2G, ollama 8G |
| **Migration safety** | PostgreSQL advisory lock pinned to single connection prevents concurrent migration races |

## Web UI

Open **http://127.0.0.1:40001** in a browser. Pages:

- **/** — Search with live HTMX-powered semantic search results
- **/artifact/{id}** — Artifact detail with summary, key ideas, entities, connections
- **/topics** — Topic lifecycle view with pagination (emerging → active → hot → cooling → dormant)
- **/digest** — Today's daily digest
- **/status** — System status (DB, NATS, ML sidecar health)
- **/settings** — Connector status and configuration

The web search uses the same semantic search engine as the API (pgvector + embedding + LLM re-ranking). Dark/light theme follows OS preference. Monochrome design — no accent colors, no emoji.

## Runtime Standards

Smackerel has a complete runtime with a repo CLI, YAML-backed config generation, a Go core (51 source files, 40 test files), a Python ML sidecar (11 files), and Docker Compose orchestration. The operational surface is standardized:

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
