package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ExpenseCategory defines an expense classification category.
type ExpenseCategory struct {
	Slug        string `json:"slug"`
	Display     string `json:"display"`
	TaxCategory string `json:"tax_category"`
}

// Config holds all configuration values for smackerel-core.
type Config struct {
	DatabaseURL string
	NATSURL     string
	LLMProvider string
	LLMModel    string
	LLMAPIKey   string
	AuthToken   string
	// Environment is the deployment environment (allowed: development | test |
	// production). Sourced from runtime.environment in smackerel.yaml via
	// SMACKEREL_ENV. MIT-040-S-004 — when "production", services fail-loud on
	// empty AuthToken at startup; in "development" or "test" the empty-token
	// dev-mode warn-and-continue is preserved.
	Environment      string
	TelegramBotToken string
	TelegramChatIDs  []string
	// Spec 044 Scope 03 — chat_id → user_id mapping for the Telegram
	// bridge. Sourced from TELEGRAM_USER_MAPPING ("<chat_id>:<user>"
	// pairs, comma-separated) via scripts/commands/config.sh which reads
	// telegram.user_mapping from smackerel.yaml. Empty in dev/test;
	// REQUIRED in production for any chat that should successfully
	// capture (production drops messages from unmapped chats).
	TelegramUserMapping map[int64]string
	OllamaURL           string
	OllamaModel         string
	EmbeddingModel      string
	DigestCron          string
	LogLevel            string
	Port                string
	MLSidecarURL        string
	CoreAPIURL          string

	// DB pool sizing (SST-compliant — from smackerel.yaml via config generate)
	DBMaxConns int32
	DBMinConns int32

	// Shutdown timeout in seconds for graceful shutdown (SST-compliant)
	ShutdownTimeoutS int

	// ML sidecar health cache TTL in seconds (SST-compliant)
	MLHealthCacheTTLS int

	// ML sidecar readiness timeout in seconds (SST-compliant)
	// Core blocks at startup until ML sidecar is healthy or timeout elapses.
	MLReadinessTimeoutS int

	// Optional connector path fields (SST-compliant — read from env, sourced from smackerel.yaml)
	BookmarksImportDir                           string
	BookmarksEnabled                             bool
	BookmarksSyncSchedule                        string
	BookmarksWatchInterval                       string
	BookmarksArchiveProcessed                    bool
	BookmarksProcessingTier                      string
	BookmarksMinURLLength                        int
	BookmarksExcludeDomains                      string
	BrowserHistoryEnabled                        bool
	BrowserHistoryPath                           string
	BrowserHistorySyncSchedule                   string
	BrowserHistoryAccessStrategy                 string
	BrowserHistoryInitialLookbackDays            int
	BrowserHistoryDwellFullMin                   string
	BrowserHistoryDwellStandardMin               string
	BrowserHistoryDwellLightMin                  string
	BrowserHistoryRepeatVisitWindow              string
	BrowserHistoryRepeatVisitThreshold           int
	BrowserHistoryContentFetchTimeout            string
	BrowserHistoryContentFetchConcurrency        int
	BrowserHistoryContentFetchDomainDelay        string
	BrowserHistoryCustomSkipDomains              string
	BrowserHistorySocialMediaIndividualThreshold string
	MapsEnabled                                  bool
	MapsImportDir                                string

	// Telegram assembly config (SST-compliant — from smackerel.yaml via config generate)
	TelegramAssemblyWindowSeconds        int
	TelegramAssemblyMaxMessages          int
	TelegramMediaGroupWindowSeconds      int
	TelegramDisambiguationTimeoutSeconds int

	// Knowledge layer config (SST-compliant — from smackerel.yaml via config generate)
	KnowledgeEnabled                        bool
	KnowledgeSynthesisTimeoutSeconds        int
	KnowledgeLintCron                       string
	KnowledgeLintStaleDays                  int
	KnowledgeConceptMaxTokens               int
	KnowledgeConceptSearchThreshold         float64
	KnowledgeCrossSourceConfidenceThreshold float64
	KnowledgeMaxSynthesisRetries            int
	KnowledgePromptContractIngestSynthesis  string
	KnowledgePromptContractCrossSource      string
	KnowledgePromptContractLintAudit        string
	KnowledgePromptContractQueryAugment     string
	KnowledgePromptContractDigestAssembly   string

	// Prompt contracts directory (SST-compliant — from smackerel.yaml via config generate)
	PromptContractsDir string

	// Observability config (SST-compliant — from smackerel.yaml via config generate)
	OTELEnabled          bool
	OTELExporterEndpoint string

	// Expense tracking config (SST-compliant — from smackerel.yaml via config generate)
	ExpensesEnabled                       bool
	ExpensesDefaultCurrency               string
	ExpensesExportMaxRows                 int
	ExpensesExportQBDateFormat            string
	ExpensesExportStdDateFormat           string
	ExpensesSuggestionsMinConfidence      float64
	ExpensesSuggestionsMinPastBusiness    int
	ExpensesSuggestionsMaxPerDigest       int
	ExpensesSuggestionsReclassifyBatchLim int
	ExpensesVendorCacheSize               int
	ExpensesDigestMaxWords                int
	ExpensesDigestNeedsReviewLimit        int
	ExpensesDigestMissingReceiptLookback  int
	IMAPExpenseLabels                     map[string]string
	ExpensesBusinessVendors               []string
	ExpensesCategories                    []ExpenseCategory

	// Telegram cook session config (SST-compliant — from smackerel.yaml via config generate)
	TelegramCookSessionTimeoutMinutes int
	TelegramCookSessionMaxPerChat     int

	// Meal planning config (SST-compliant — from smackerel.yaml via config generate)
	MealPlanEnabled          bool
	MealPlanDefaultServings  int
	MealPlanMealTypes        []string
	MealPlanMealTimes        map[string]string
	MealPlanCalendarSync     bool
	MealPlanAutoComplete     bool
	MealPlanAutoCompleteCron string

	// Card rewards config (Spec 083 — SST-compliant, from smackerel.yaml via config generate)
	CardRewards CardRewardsConfig

	// Connector enable/credential/schedule fields (SST-compliant — from smackerel.yaml via config generate)
	MapsSyncSchedule              string
	MapsWatchInterval             string
	MapsArchiveProcessed          bool
	MapsHomeDetection             string
	MapsCommuteWeekdaysOnly       bool
	MapsMinDistanceM              float64
	MapsMinDurationMin            float64
	MapsLocationRadiusM           float64
	MapsCommuteMinOccurrences     float64
	MapsCommuteWindowDays         float64
	MapsTripMinDistanceKm         float64
	MapsTripMinOvernightHours     float64
	MapsLinkTimeExtendMin         float64
	MapsLinkProximityRadiusM      float64
	DiscordEnabled                bool
	DiscordBotToken               string
	DiscordSyncSchedule           string
	DiscordEnableGateway          bool
	DiscordBackfillLimit          float64
	DiscordIncludeThreads         bool
	DiscordIncludePins            bool
	DiscordCaptureCommands        []interface{}
	DiscordMonitoredChannels      []interface{}
	TwitterEnabled                bool
	TwitterBearerToken            string
	TwitterOAuthClientID          string
	TwitterOAuthClientSecret      string
	TwitterOAuthRedirectURL       string
	TwitterSyncSchedule           string
	TwitterSyncMode               string
	TwitterArchiveDir             string
	WeatherEnabled                bool
	WeatherSyncSchedule           string
	WeatherLocations              []interface{}
	WeatherEnableAlerts           bool
	WeatherForecastDays           float64
	WeatherPrecision              float64
	GovAlertsEnabled              bool
	GovAlertsSyncSchedule         string
	GovAlertsAirnowAPIKey         string
	GovAlertsLocations            []interface{}
	GovAlertsMinEarthquakeMag     float64
	GovAlertsTravelLocations      []interface{}
	GovAlertsSourceEarthquake     bool
	GovAlertsSourceWeather        bool
	GovAlertsSourceTsunami        bool
	GovAlertsSourceVolcano        bool
	GovAlertsSourceWildfire       bool
	GovAlertsSourceAirnow         bool
	GovAlertsSourceGdacs          bool
	FinancialMarketsEnabled       bool
	FinancialMarketsSyncSchedule  string
	FinancialMarketsFinnhubAPIKey string
	FinancialMarketsFredAPIKey    string
	FinancialMarketsWatchlist     map[string]interface{}
	FinancialMarketsAlertThresh   float64
	FinancialMarketsCoingecko     bool
	FinancialMarketsFredEnabled   bool
	FinancialMarketsFredSeries    []interface{}
	IMAPSyncSchedule              string
	CalDAVSyncSchedule            string
	YouTubeSyncSchedule           string

	// Hospitable connector (SST-compliant — from smackerel.yaml via config generate)
	HospitableEnabled             bool
	HospitableAccessToken         string
	HospitableBaseURL             string
	HospitableSyncSchedule        string
	HospitableInitialLookbackDays int
	HospitablePageSize            int
	HospitableSyncProperties      bool
	HospitableSyncReservations    bool
	HospitableSyncMessages        bool
	HospitableSyncReviews         bool
	HospitableTierMessages        string
	HospitableTierReviews         string
	HospitableTierReservations    string
	HospitableTierProperties      string

	// GuestHost connector (SST-compliant — from smackerel.yaml via config generate)
	GuestHostEnabled      bool
	GuestHostBaseURL      string
	GuestHostAPIKey       string
	GuestHostSyncSchedule string
	GuestHostEventTypes   string

	// QF decisions connector (SST-compliant — from smackerel.yaml via config generate)
	QFDecisionsEnabled                 bool
	QFDecisionsBaseURL                 string
	QFDecisionsCredentialRef           string
	QFDecisionsSyncSchedule            string
	QFDecisionsPacketVersion           int
	QFDecisionsPageSize                int
	QFDecisionsCallbackSigningKeysJSON string

	// CORS allowed origins (SST-compliant — from smackerel.yaml via config generate)
	CORSAllowedOrigins []string

	// BUG-020-009 — per-call HTTP client timeouts (seconds). SST
	// zero-defaults: each MUST be > 0; fail-loud at Load() (via
	// mustParseIntEnv → intLoadErrs) and at Validate() (range check).
	// FinancialMarketsHTTPTimeoutSeconds replaces the literal at
	// internal/connector/markets/markets.go (was `Timeout: 10 * time.Second`).
	// AuthOAuthHTTPTimeoutSeconds replaces the literal at
	// internal/auth/oauth.go (was `Timeout: 15 * time.Second`).
	FinancialMarketsHTTPTimeoutSeconds int
	AuthOAuthHTTPTimeoutSeconds        int

	// Runtime trusted reverse-proxy CIDR allowlist (BUG-020-005,
	// F-SEC-R30-001). When the connecting TCP peer is in one of these
	// CIDRs, the API trusts X-Forwarded-For / X-Real-IP / True-Client-IP
	// headers and rewrites r.RemoteAddr accordingly (so httprate.LimitByIP
	// and slog see the real client). Empty slice = secure-by-default:
	// any caller that can send HTTP headers cannot bypass per-IP rate
	// limits or forge `remote_addr` log fields. Source: runtime.trusted_proxies
	// in smackerel.yaml → RUNTIME_TRUSTED_PROXIES (comma-separated CIDRs).
	RuntimeTrustedProxies []string

	// Shared typed config blocks (SST-compliant — from smackerel.yaml via config generate)
	Drive           DriveConfig
	Photos          PhotosConfig
	Recommendations RecommendationsConfig
	Notification    NotificationConfig
	NtfySourcesJSON string

	// Spec 058 Scope 1 — Chrome Extension Bridge ingest config.
	// Sourced from `extension.ingest.*` in config/smackerel.yaml via
	// EXTENSION_INGEST_* env vars; fail-loud SST per smackerel-no-defaults.
	Extension ExtensionConfig

	// Spec 021 Scope 4 — Unified surfacing controller config.
	// Sourced from `surfacing.*` in config/smackerel.yaml via
	// SURFACING_* env vars; fail-loud SST per smackerel-no-defaults.
	Surfacing SurfacingConfig

	// Spec 080 SCOPE-080-01 — Knowledge Graph Public API SST envelope.
	// Sourced from `knowledge_graph_api.*` in config/smackerel.yaml
	// via KNOWLEDGE_GRAPH_API_* env vars; fail-loud SST per
	// smackerel-no-defaults. Consumed by internal/api/graphapi at
	// handler-mount time (scopes 02-04).
	KnowledgeGraphAPI KnowledgeGraphAPIConfig

	// BUG-020-008 — fail-loud int env parsing. Populated by Load() after
	// the cfg literal initializes; each entry is the error string produced
	// by mustParseIntEnv for one of the 8 SST-required int env vars
	// (BOOKMARKS_MIN_URL_LENGTH, BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS,
	// BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD,
	// BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY, QF_DECISIONS_PACKET_VERSION,
	// QF_DECISIONS_PAGE_SIZE, HOSPITABLE_INITIAL_LOOKBACK_DAYS,
	// HOSPITABLE_PAGE_SIZE). Validate() folds these into the same
	// consolidated missing-keys error as requiredVars() so a single boot
	// surfaces every offending key in one message instead of silently
	// substituting 0.
	intLoadErrs []string

	// Spec 044 — Per-user bearer auth foundation. SST-compliant; populated
	// from AUTH_* env vars produced by `./smackerel.sh config generate`.
	// Empty signing/hashing/bootstrap fields are accepted in dev/test
	// (preserves SMACKEREL_AUTH_TOKEN ergonomic) but rejected at startup
	// when Environment == "production" AND Auth.Enabled == true.
	Auth AuthConfig

	// Spec 061 — Conversational Assistant (Transport-Agnostic) SST
	// envelope. Populated by loadAssistantConfig() at the tail of Load()
	// from ASSISTANT_* env vars emitted by `./smackerel.sh config generate`.
	// Every field is REQUIRED; missing keys fail loud at Load() with the
	// [F061-SST-MISSING] prefix. Rule-based startup validation runs in
	// validateAssistantConfig() from Validate().
	Assistant AssistantConfig

	// Spec 074 SCOPE-1 — capture-as-fallback policy SST. Populated by
	// LoadCaptureFallback() at the tail of Load().
	CaptureFallback CaptureFallbackConfig

	// Spec 095 SCOPE-01 — Retrieval-Strategy Routing + Freshness-Aware
	// Retrieval SST. Populated by LoadRetrieval() at the tail of Load()
	// from RETRIEVAL_* env vars produced by `./smackerel.sh config
	// generate`. Every field is REQUIRED; missing/invalid keys fail loud
	// at Load() with the [F095-SST-MISSING] prefix. Validation is
	// unconditional (no Enabled short-circuit) per design §10.
	Retrieval RetrievalConfig

	// Spec 096 SCOPE-01 — Multi-Provider Model Connection registry SST.
	// Populated by LoadModelConnections() at the tail of Load() from the
	// LLM_CONNECTIONS_JSON / LLM_DISCOVERY_* / LLM_MODEL_COSTS_JSON env
	// vars produced by `./smackerel.sh config generate`. The closed-set
	// loader aborts fail-loud ([F096-SST-MISSING] / [F096-SST-INVALID]) on
	// a missing env var, an unknown kind, a missing required per-kind
	// param, a non-positive discovery bound, an env-mode secret absent
	// from infrastructure.secret_keys, or an enabled non-ollama model with
	// no model_costs entry (G028, no silent default / no Ollama fallback).
	ModelConnections ModelConnectionsConfig

	// Spec 075 — legacy retirement window SST. Populated by
	// LoadLegacyRetirement() at the tail of Load().
	LegacyRetirement LegacyRetirementConfig

	// Spec 045 FR-045-001 / FR-045-002 — deploy resource envelope and ML
	// model memory profile. SST-compliant; populated from
	// {SERVICE}_CPU_LIMIT, {SERVICE}_MEMORY_LIMIT, and
	// ML_MODEL_MEMORY_PROFILES_JSON env vars produced by
	// `./smackerel.sh config generate`. The Validate() chain rejects any
	// configured ML model whose profile memory_mib exceeds
	// MLMemoryLimitMiB (parsed from ML_MEMORY_LIMIT) and rejects
	// configured models that have no profile entry. Hand-edited compose
	// memory/cpu literals are forbidden — the
	// internal/deploy/compose_contract_test.go assertResourceContract()
	// blocks regression at build time.
	PostgresCPULimit     string
	PostgresMemoryLimit  string
	NATSCPULimit         string
	NATSMemoryLimit      string
	CoreCPULimit         string
	CoreMemoryLimit      string
	MLCPULimit           string
	MLMemoryLimit        string // raw compose-style string, e.g. "3G"
	MLMemoryLimitMiB     int    // parsed from MLMemoryLimit
	OllamaCPULimit       string
	OllamaMemoryLimit    string // raw compose-style string, e.g. "8G"
	OllamaMemoryLimitMiB int    // parsed from OllamaMemoryLimit
	// Spec 082 SCOPE-082-02 — raw OLLAMA_KEEP_ALIVE value. The
	// concurrent-envelope guard (validateModelEnvelopes) only enforces
	// the interactive-set SUM against the ollama envelope when keep-alive
	// keeps loaded models resident (see ollamaKeepAliveResident).
	OllamaKeepAlive       string
	MLModelMemoryProfiles map[string]int // model name → required MiB

	// BUG-045-001 — Per-service envelope routing for model env vars.
	// The 15 fields below name every ollama-routed model env var the
	// SST emits today plus three forward-compatible env vars
	// (OLLAMA_OCR_MODEL, OLLAMA_REASONING_MODEL, OLLAMA_FAST_MODEL)
	// that DD-3 enumerates but scripts/commands/config.sh does NOT
	// currently emit; non-emitted env vars resolve to empty via
	// os.Getenv() and are skipped by the validator's skip-empty branch.
	// validateModelEnvelopes() routes every non-empty value in this
	// bucket against c.OllamaMemoryLimitMiB.
	OllamaVisionModel                  string
	OllamaOcrModel                     string
	OllamaReasoningModel               string
	OllamaFastModel                    string
	PhotosIntelligenceClassifyModel    string
	PhotosIntelligenceSensitivityModel string
	PhotosIntelligenceAestheticModel   string
	PhotosIntelligenceOcrModel         string
	AgentProviderDefaultModel          string
	AgentProviderReasoningModel        string
	AgentProviderFastModel             string
	AgentProviderVisionModel           string
	AgentProviderOcrModel              string
	// The 1 field below names the ml-sidecar-routed image-embedding
	// model emitted by scripts/commands/config.sh. EMBEDDING_MODEL is
	// already in the Config struct (text-embedding route); the photos
	// pipeline's embed_model loads in the smackerel_ml container and
	// must route against c.MLMemoryLimitMiB.
	PhotosIntelligenceEmbedModel string

	// Spec 046 FR-046-001 / FR-046-002 / FR-046-003 — NATS production
	// hardening. SST-compliant; populated from NATS_* env vars produced
	// by `./smackerel.sh config generate`. The Validate() chain rejects
	// missing or out-of-range values. NATSStreamMaxBytes MUST contain a
	// positive entry for every stream returned by
	// internal/nats.AllStreams(); internal/nats.Client.EnsureStreams
	// fails loud if any stream is absent so unbounded streams are
	// caught at startup, not after disk is full.
	//
	// The Raw* fields hold the on-the-wire env strings so requiredVars()
	// can emit one consolidated missing-key error in the existing
	// fail-loud SST pattern; the typed fields below are used at runtime.
	NATSMaxReconnectAttemptsRaw  string
	NATSReconnectTimeWaitSecsRaw string
	NATSMaxPayloadBytesRaw       string
	NATSMaxFileStoreBytesRaw     string
	NATSMaxMemStoreBytesRaw      string
	NATSStreamMaxBytesJSON       string
	NATSMaxReconnectAttempts     int
	NATSReconnectTimeWaitSecs    int
	NATSMaxPayloadBytes          int64
	NATSMaxFileStoreBytes        int64
	NATSMaxMemStoreBytes         int64
	NATSStreamMaxBytes           map[string]int64

	// Spec 081 FR-081-001 / FR-081-002 — Python sidecar consumer
	// contract. Go core does not consume these at runtime (the
	// ml/app/nats_client.py reads them directly via os.environ), but
	// they are SST-required so the requiredVars()/Validate() path
	// fail-loud at boot if the env file is missing them.
	NATSConsumerMaxDeliverRaw     string
	NATSConsumerAckWaitSecondsRaw string

	// Spec 048 — Backup and Restore Automation. SST-compliant; populated
	// from BACKUP_* env vars produced by `./smackerel.sh config generate`.
	// BackupLocalDir is the host-side staging directory `./smackerel.sh
	// backup` writes pg_dump artifacts to before a deploy adapter ships
	// them off-host. BackupStatusFile is the JSON status the script
	// updates after every run; the Go core's internal/backup.Watcher
	// polls it and republishes `smackerel_backup_*` metrics so spec 049
	// SmackerelBackupStale alert can fire.
	//
	// BackupRetentionDaily / BackupRetentionWeekly encode the product
	// retention contract (FR-048-001 default: 7 daily + 4 weekly). All
	// four fields are REQUIRED at the loader boundary — Gate G028
	// fail-loud forbids hidden defaults.
	//
	// BackupWatcherPollSeconds is how often the metrics watcher polls
	// the status file. 60s is the recommended default; lower values
	// reduce the stale-metric window but cost more CPU.
	BackupLocalDir           string
	BackupStatusFile         string
	BackupRetentionDailyRaw  string
	BackupRetentionWeeklyRaw string
	BackupWatcherPollSecsRaw string
	BackupRetentionDaily     int
	BackupRetentionWeekly    int
	BackupWatcherPollSecs    int
}

// AuthConfig holds the SST-resolved per-user bearer-auth subsystem
// configuration (spec 044). Every field is REQUIRED at the generator
// boundary; secret-bearing fields (SigningActivePrivateKey,
// SigningActiveKeyID, AtRestHashingKey, BootstrapToken) are accepted as
// empty strings in dev/test and validated as non-empty in production by
// `cmd/core/wiring.go` startup validation.
type AuthConfig struct {
	// Enabled gates the per-user PASETO validation path. False in dev/test
	// by default; true in production-class environments via
	// environments.<env>.auth_enabled override. SCN-AUTH-005/011 preserve
	// shared SMACKEREL_AUTH_TOKEN semantics when Enabled is false.
	Enabled bool

	// TokenFormat is the wire-format identifier for issued tokens. Spec 044
	// hardcodes "paseto-v4-public" (OQ-1 RESOLVED).
	TokenFormat string

	// Signing key material. SigningActivePrivateKey is the Ed25519 private
	// key that signs newly issued tokens; SigningActiveKeyID is the short
	// identifier embedded in the kid claim. SigningPriorPublicKey + KeyID
	// are populated during the rotation grace window so in-flight tokens
	// continue to validate. Empty values are valid in dev/test only.
	SigningActivePrivateKey string
	SigningActiveKeyID      string
	SigningPriorPublicKey   string
	SigningPriorKeyID       string

	// TokenTTLHours bounds the lifetime of an issued token before requiring
	// rotation. MUST be > 0; design default is 720 hours (30 days).
	TokenTTLHours int

	// RotationGraceWindowHours determines how long the prior token + prior
	// signing key remain valid after rotation. NFR-AUTH-003 floor: ≥ 24.
	RotationGraceWindowHours int

	// ClockSkewToleranceSeconds — NFR-AUTH-005 ceiling: ≤ 60.
	ClockSkewToleranceSeconds int

	// RevocationCacheRefreshIntervalSeconds — periodic DB poll cadence as
	// the failure-mode backstop when the NATS broadcast channel is down.
	// Worst-case revocation propagation is bounded by NFR-AUTH-006 (≤ 60s).
	RevocationCacheRefreshIntervalSeconds int

	// RevocationNATSSubject is the cross-instance broadcast channel for
	// token revocation events. Default "auth.revocations" per design §4.
	RevocationNATSSubject string

	// AtRestHashingKey is the HMAC-SHA-256 key used to hash issued tokens
	// before persistence. MUST be empty in dev/test only.
	AtRestHashingKey string

	// ProductionSharedTokenFallbackEnabled is the OQ-5 escape hatch that
	// lets the legacy SMACKEREL_AUTH_TOKEN authenticate in production
	// during the migration window. Defaults to false; flipping to true
	// emits a deprecation warning on every successful match.
	ProductionSharedTokenFallbackEnabled bool

	// TelemetryEnabled gates emission of smackerel_auth_* Prometheus
	// metrics (spec 030 dashboards via OQ-9).
	TelemetryEnabled bool

	// TelemetryMetricPrefix prefixes all auth subsystem metric names.
	// Default "smackerel_auth"; tied to spec 030 collector naming.
	TelemetryMetricPrefix string

	// BootstrapToken authorizes first-user enrollment on a fresh production
	// deployment. Consumed exactly once via `./smackerel.sh auth bootstrap`
	// and then cleared by the operator (OQ-10 RESOLVED).
	BootstrapToken string

	// WebRegistrationInviteToken is the spec-091 self-registration gate
	// secret (WEB_REGISTRATION_INVITE_TOKEN). OPTIONAL: empty ⇒ POST
	// /v1/web/register is disabled (fail-loud at POST, never open signup).
	// Unlike BootstrapToken it is repeatable and NOT production-required, so
	// loadAuthConfig does NOT append it to authErrors. Compared in constant
	// time by HandleWebRegister; never logged.
	WebRegistrationInviteToken string
}

// Load reads configuration from environment variables.
// It returns an error naming every missing required variable.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		NATSURL:          os.Getenv("NATS_URL"),
		LLMProvider:      os.Getenv("LLM_PROVIDER"),
		LLMModel:         os.Getenv("LLM_MODEL"),
		LLMAPIKey:        os.Getenv("LLM_API_KEY"),
		AuthToken:        os.Getenv("SMACKEREL_AUTH_TOKEN"),
		Environment:      os.Getenv("SMACKEREL_ENV"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		OllamaURL:        os.Getenv("OLLAMA_URL"),
		OllamaModel:      os.Getenv("OLLAMA_MODEL"),
		EmbeddingModel:   os.Getenv("EMBEDDING_MODEL"),
		DigestCron:       os.Getenv("DIGEST_CRON"),
		LogLevel:         os.Getenv("LOG_LEVEL"),
		Port:             os.Getenv("PORT"),
		MLSidecarURL:     os.Getenv("ML_SIDECAR_URL"),
		CoreAPIURL:       os.Getenv("CORE_API_URL"),

		BookmarksImportDir:                           os.Getenv("BOOKMARKS_IMPORT_DIR"),
		BookmarksEnabled:                             os.Getenv("BOOKMARKS_ENABLED") == "true",
		BookmarksSyncSchedule:                        os.Getenv("BOOKMARKS_SYNC_SCHEDULE"),
		BookmarksWatchInterval:                       os.Getenv("BOOKMARKS_WATCH_INTERVAL"),
		BookmarksArchiveProcessed:                    os.Getenv("BOOKMARKS_ARCHIVE_PROCESSED") == "true",
		BookmarksProcessingTier:                      os.Getenv("BOOKMARKS_PROCESSING_TIER"),
		BookmarksExcludeDomains:                      os.Getenv("BOOKMARKS_EXCLUDE_DOMAINS"),
		BrowserHistoryEnabled:                        os.Getenv("BROWSER_HISTORY_ENABLED") == "true",
		BrowserHistoryPath:                           os.Getenv("BROWSER_HISTORY_PATH"),
		BrowserHistorySyncSchedule:                   os.Getenv("BROWSER_HISTORY_SYNC_SCHEDULE"),
		BrowserHistoryAccessStrategy:                 os.Getenv("BROWSER_HISTORY_ACCESS_STRATEGY"),
		BrowserHistoryDwellFullMin:                   os.Getenv("BROWSER_HISTORY_DWELL_FULL_MIN"),
		BrowserHistoryDwellStandardMin:               os.Getenv("BROWSER_HISTORY_DWELL_STANDARD_MIN"),
		BrowserHistoryDwellLightMin:                  os.Getenv("BROWSER_HISTORY_DWELL_LIGHT_MIN"),
		BrowserHistoryRepeatVisitWindow:              os.Getenv("BROWSER_HISTORY_REPEAT_VISIT_WINDOW"),
		BrowserHistoryContentFetchTimeout:            os.Getenv("BROWSER_HISTORY_CONTENT_FETCH_TIMEOUT"),
		BrowserHistoryContentFetchDomainDelay:        os.Getenv("BROWSER_HISTORY_CONTENT_FETCH_DOMAIN_DELAY"),
		BrowserHistoryCustomSkipDomains:              os.Getenv("BROWSER_HISTORY_CUSTOM_SKIP_DOMAINS"),
		BrowserHistorySocialMediaIndividualThreshold: os.Getenv("BROWSER_HISTORY_SOCIAL_MEDIA_INDIVIDUAL_THRESHOLD"),
		MapsEnabled:                                  os.Getenv("MAPS_ENABLED") == "true",
		MapsImportDir:                                os.Getenv("MAPS_IMPORT_DIR"),

		// Connector enable/credential/schedule (SST-compliant)
		MapsSyncSchedule:              os.Getenv("MAPS_SYNC_SCHEDULE"),
		MapsWatchInterval:             os.Getenv("MAPS_WATCH_INTERVAL"),
		MapsArchiveProcessed:          os.Getenv("MAPS_ARCHIVE_PROCESSED") == "true",
		MapsHomeDetection:             os.Getenv("MAPS_HOME_DETECTION"),
		MapsCommuteWeekdaysOnly:       os.Getenv("MAPS_COMMUTE_WEEKDAYS_ONLY") == "true",
		MapsMinDistanceM:              parseEnvFloat("MAPS_MIN_DISTANCE_M"),
		MapsMinDurationMin:            parseEnvFloat("MAPS_MIN_DURATION_MIN"),
		MapsLocationRadiusM:           parseEnvFloat("MAPS_LOCATION_RADIUS_M"),
		MapsCommuteMinOccurrences:     parseEnvFloat("MAPS_COMMUTE_MIN_OCCURRENCES"),
		MapsCommuteWindowDays:         parseEnvFloat("MAPS_COMMUTE_WINDOW_DAYS"),
		MapsTripMinDistanceKm:         parseEnvFloat("MAPS_TRIP_MIN_DISTANCE_KM"),
		MapsTripMinOvernightHours:     parseEnvFloat("MAPS_TRIP_MIN_OVERNIGHT_HOURS"),
		MapsLinkTimeExtendMin:         parseEnvFloat("MAPS_LINK_TIME_EXTEND_MIN"),
		MapsLinkProximityRadiusM:      parseEnvFloat("MAPS_LINK_PROXIMITY_RADIUS_M"),
		DiscordEnabled:                os.Getenv("DISCORD_ENABLED") == "true",
		DiscordBotToken:               os.Getenv("DISCORD_BOT_TOKEN"),
		DiscordSyncSchedule:           os.Getenv("DISCORD_SYNC_SCHEDULE"),
		DiscordEnableGateway:          os.Getenv("DISCORD_ENABLE_GATEWAY") == "true",
		DiscordBackfillLimit:          parseEnvFloat("DISCORD_BACKFILL_LIMIT"),
		DiscordIncludeThreads:         os.Getenv("DISCORD_INCLUDE_THREADS") == "true",
		DiscordIncludePins:            os.Getenv("DISCORD_INCLUDE_PINS") == "true",
		DiscordCaptureCommands:        parseEnvJSONArray("DISCORD_CAPTURE_COMMANDS"),
		DiscordMonitoredChannels:      parseEnvJSONArray("DISCORD_MONITORED_CHANNELS"),
		TwitterEnabled:                os.Getenv("TWITTER_ENABLED") == "true",
		TwitterBearerToken:            os.Getenv("TWITTER_BEARER_TOKEN"),
		TwitterOAuthClientID:          os.Getenv("TWITTER_OAUTH_CLIENT_ID"),
		TwitterOAuthClientSecret:      os.Getenv("TWITTER_OAUTH_CLIENT_SECRET"),
		TwitterOAuthRedirectURL:       os.Getenv("TWITTER_OAUTH_REDIRECT_URL"),
		TwitterSyncSchedule:           os.Getenv("TWITTER_SYNC_SCHEDULE"),
		TwitterSyncMode:               os.Getenv("TWITTER_SYNC_MODE"),
		TwitterArchiveDir:             os.Getenv("TWITTER_ARCHIVE_DIR"),
		WeatherEnabled:                os.Getenv("WEATHER_ENABLED") == "true",
		WeatherSyncSchedule:           os.Getenv("WEATHER_SYNC_SCHEDULE"),
		WeatherLocations:              parseEnvJSONArray("WEATHER_LOCATIONS"),
		WeatherEnableAlerts:           os.Getenv("WEATHER_ENABLE_ALERTS") == "true",
		WeatherForecastDays:           parseEnvFloat("WEATHER_FORECAST_DAYS"),
		WeatherPrecision:              parseEnvFloat("WEATHER_PRECISION"),
		GovAlertsEnabled:              os.Getenv("GOV_ALERTS_ENABLED") == "true",
		GovAlertsSyncSchedule:         os.Getenv("GOV_ALERTS_SYNC_SCHEDULE"),
		GovAlertsAirnowAPIKey:         os.Getenv("GOV_ALERTS_AIRNOW_API_KEY"),
		GovAlertsLocations:            parseEnvJSONArray("GOV_ALERTS_LOCATIONS"),
		GovAlertsMinEarthquakeMag:     parseEnvFloat("GOV_ALERTS_MIN_EARTHQUAKE_MAG"),
		GovAlertsTravelLocations:      parseEnvJSONArray("GOV_ALERTS_TRAVEL_LOCATIONS"),
		GovAlertsSourceEarthquake:     os.Getenv("GOV_ALERTS_SOURCE_EARTHQUAKE") == "true",
		GovAlertsSourceWeather:        os.Getenv("GOV_ALERTS_SOURCE_WEATHER") == "true",
		GovAlertsSourceTsunami:        os.Getenv("GOV_ALERTS_SOURCE_TSUNAMI") == "true",
		GovAlertsSourceVolcano:        os.Getenv("GOV_ALERTS_SOURCE_VOLCANO") == "true",
		GovAlertsSourceWildfire:       os.Getenv("GOV_ALERTS_SOURCE_WILDFIRE") == "true",
		GovAlertsSourceAirnow:         os.Getenv("GOV_ALERTS_SOURCE_AIRNOW") == "true",
		GovAlertsSourceGdacs:          os.Getenv("GOV_ALERTS_SOURCE_GDACS") == "true",
		FinancialMarketsEnabled:       os.Getenv("FINANCIAL_MARKETS_ENABLED") == "true",
		FinancialMarketsSyncSchedule:  os.Getenv("FINANCIAL_MARKETS_SYNC_SCHEDULE"),
		FinancialMarketsFinnhubAPIKey: os.Getenv("FINANCIAL_MARKETS_FINNHUB_API_KEY"),
		FinancialMarketsFredAPIKey:    os.Getenv("FINANCIAL_MARKETS_FRED_API_KEY"),
		FinancialMarketsWatchlist:     parseEnvJSONObject("FINANCIAL_MARKETS_WATCHLIST"),
		FinancialMarketsAlertThresh:   parseEnvFloat("FINANCIAL_MARKETS_ALERT_THRESHOLD"),
		FinancialMarketsCoingecko:     os.Getenv("FINANCIAL_MARKETS_COINGECKO_ENABLED") == "true",
		FinancialMarketsFredEnabled:   os.Getenv("FINANCIAL_MARKETS_FRED_ENABLED") == "true",
		FinancialMarketsFredSeries:    parseEnvJSONArray("FINANCIAL_MARKETS_FRED_SERIES"),
		IMAPSyncSchedule:              os.Getenv("IMAP_SYNC_SCHEDULE"),
		CalDAVSyncSchedule:            os.Getenv("CALDAV_SYNC_SCHEDULE"),
		YouTubeSyncSchedule:           os.Getenv("YOUTUBE_SYNC_SCHEDULE"),

		// GuestHost connector
		GuestHostEnabled:      os.Getenv("GUESTHOST_ENABLED") == "true",
		GuestHostBaseURL:      os.Getenv("GUESTHOST_BASE_URL"),
		GuestHostAPIKey:       os.Getenv("GUESTHOST_API_KEY"),
		GuestHostSyncSchedule: os.Getenv("GUESTHOST_SYNC_SCHEDULE"),
		GuestHostEventTypes:   os.Getenv("GUESTHOST_EVENT_TYPES"),

		// QF decisions connector
		QFDecisionsEnabled:       os.Getenv("QF_DECISIONS_ENABLED") == "true",
		QFDecisionsBaseURL:       os.Getenv("QF_DECISIONS_BASE_URL"),
		QFDecisionsCredentialRef: os.Getenv("QF_DECISIONS_CREDENTIAL_REF"),
		QFDecisionsSyncSchedule:  os.Getenv("QF_DECISIONS_SYNC_SCHEDULE"),
		// BUG-020-010 — QF callback HMAC bridge signing keystore JSON.
		// PERMISSIVE: empty means "callback signing not configured in this
		// environment" and the connector continues to run for
		// ingest/render/evidence flows. Non-empty values are validated by
		// validateQFDecisionsConfig() at boot via
		// qfdecisions.LoadCallbackKeystoreFromJSON (fail-loud on parse).
		QFDecisionsCallbackSigningKeysJSON: os.Getenv("QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON"),

		// Hospitable connector
		HospitableEnabled:          os.Getenv("HOSPITABLE_ENABLED") == "true",
		HospitableAccessToken:      os.Getenv("HOSPITABLE_ACCESS_TOKEN"),
		HospitableBaseURL:          os.Getenv("HOSPITABLE_BASE_URL"),
		HospitableSyncSchedule:     os.Getenv("HOSPITABLE_SYNC_SCHEDULE"),
		HospitableSyncProperties:   os.Getenv("HOSPITABLE_SYNC_PROPERTIES") == "true",
		HospitableSyncReservations: os.Getenv("HOSPITABLE_SYNC_RESERVATIONS") == "true",
		HospitableSyncMessages:     os.Getenv("HOSPITABLE_SYNC_MESSAGES") == "true",
		HospitableSyncReviews:      os.Getenv("HOSPITABLE_SYNC_REVIEWS") == "true",
		HospitableTierMessages:     os.Getenv("HOSPITABLE_TIER_MESSAGES"),
		HospitableTierReviews:      os.Getenv("HOSPITABLE_TIER_REVIEWS"),
		HospitableTierReservations: os.Getenv("HOSPITABLE_TIER_RESERVATIONS"),
		HospitableTierProperties:   os.Getenv("HOSPITABLE_TIER_PROPERTIES"),

		// Spec 045 FR-045-001 / FR-045-002 — deploy resource envelope and
		// ML model memory profile. Raw env-var values are loaded here;
		// MLMemoryLimit is parsed into MiB and MLModelMemoryProfiles is
		// JSON-decoded BELOW (after the cfg literal closes) because both
		// require error handling. Validate() then enforces the envelope
		// contract.
		PostgresCPULimit:    os.Getenv("POSTGRES_CPU_LIMIT"),
		PostgresMemoryLimit: os.Getenv("POSTGRES_MEMORY_LIMIT"),
		NATSCPULimit:        os.Getenv("NATS_CPU_LIMIT"),
		NATSMemoryLimit:     os.Getenv("NATS_MEMORY_LIMIT"),
		CoreCPULimit:        os.Getenv("CORE_CPU_LIMIT"),
		CoreMemoryLimit:     os.Getenv("CORE_MEMORY_LIMIT"),
		MLCPULimit:          os.Getenv("ML_CPU_LIMIT"),
		MLMemoryLimit:       os.Getenv("ML_MEMORY_LIMIT"),
		OllamaCPULimit:      os.Getenv("OLLAMA_CPU_LIMIT"),
		OllamaMemoryLimit:   os.Getenv("OLLAMA_MEMORY_LIMIT"),
		OllamaKeepAlive:     os.Getenv("OLLAMA_KEEP_ALIVE"),

		// BUG-045-001 — Per-service envelope routing model env vars.
		// Every value below is loaded from the SST-emitted env var
		// when set (scripts/commands/config.sh emits 12 ollama-routed
		// + 2 ml-sidecar-routed vars today; OLLAMA_OCR_MODEL,
		// OLLAMA_REASONING_MODEL, OLLAMA_FAST_MODEL are forward-
		// compatible — empty today). validateModelEnvelopes() routes
		// every non-empty value against the correct service envelope.
		OllamaVisionModel:                  os.Getenv("OLLAMA_VISION_MODEL"),
		OllamaOcrModel:                     os.Getenv("OLLAMA_OCR_MODEL"),
		OllamaReasoningModel:               os.Getenv("OLLAMA_REASONING_MODEL"),
		OllamaFastModel:                    os.Getenv("OLLAMA_FAST_MODEL"),
		PhotosIntelligenceClassifyModel:    os.Getenv("PHOTOS_INTELLIGENCE_CLASSIFY_MODEL"),
		PhotosIntelligenceSensitivityModel: os.Getenv("PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL"),
		PhotosIntelligenceAestheticModel:   os.Getenv("PHOTOS_INTELLIGENCE_AESTHETIC_MODEL"),
		PhotosIntelligenceOcrModel:         os.Getenv("PHOTOS_INTELLIGENCE_OCR_MODEL"),
		AgentProviderDefaultModel:          os.Getenv("AGENT_PROVIDER_DEFAULT_MODEL"),
		AgentProviderReasoningModel:        os.Getenv("AGENT_PROVIDER_REASONING_MODEL"),
		AgentProviderFastModel:             os.Getenv("AGENT_PROVIDER_FAST_MODEL"),
		AgentProviderVisionModel:           os.Getenv("AGENT_PROVIDER_VISION_MODEL"),
		AgentProviderOcrModel:              os.Getenv("AGENT_PROVIDER_OCR_MODEL"),
		PhotosIntelligenceEmbedModel:       os.Getenv("PHOTOS_INTELLIGENCE_EMBED_MODEL"),

		// Spec 046 — NATS production hardening (raw env strings; parsed below).
		NATSMaxReconnectAttemptsRaw:  os.Getenv("NATS_MAX_RECONNECT_ATTEMPTS"),
		NATSReconnectTimeWaitSecsRaw: os.Getenv("NATS_RECONNECT_TIME_WAIT_SECONDS"),
		NATSMaxPayloadBytesRaw:       os.Getenv("NATS_MAX_PAYLOAD_BYTES"),
		NATSMaxFileStoreBytesRaw:     os.Getenv("NATS_MAX_FILE_STORE_BYTES"),
		NATSMaxMemStoreBytesRaw:      os.Getenv("NATS_MAX_MEM_STORE_BYTES"),
		NATSStreamMaxBytesJSON:       os.Getenv("NATS_STREAM_MAX_BYTES_JSON"),

		// Spec 081 — Python sidecar consumer contract (SST-required).
		NATSConsumerMaxDeliverRaw:     os.Getenv("NATS_CONSUMER_MAX_DELIVER"),
		NATSConsumerAckWaitSecondsRaw: os.Getenv("NATS_CONSUMER_ACK_WAIT_SECONDS"),

		// Spec 048 — Backup and Restore Automation. Raw env strings
		// here; integer parsing happens below so the loader can fail
		// loud with a precise error naming the offending key.
		BackupLocalDir:           os.Getenv("BACKUP_LOCAL_DIR"),
		BackupStatusFile:         os.Getenv("BACKUP_STATUS_FILE"),
		BackupRetentionDailyRaw:  os.Getenv("BACKUP_RETENTION_DAILY"),
		BackupRetentionWeeklyRaw: os.Getenv("BACKUP_RETENTION_WEEKLY"),
		BackupWatcherPollSecsRaw: os.Getenv("BACKUP_WATCHER_POLL_SECONDS"),
		NtfySourcesJSON:          os.Getenv("NTFY_SOURCES_JSON"),
	}

	// BUG-020-008 — populate the 8 SST-required int env vars via the
	// fail-loud mustParseIntEnv helper. Errors accumulate into
	// cfg.intLoadErrs so Validate() can fold them into the single
	// consolidated missing-keys error rather than failing on the first
	// offender. This mirrors the requiredVars() pattern used by every
	// other SST-required key.
	intFields := []struct {
		key string
		dst *int
	}{
		{"BOOKMARKS_MIN_URL_LENGTH", &cfg.BookmarksMinURLLength},
		{"BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS", &cfg.BrowserHistoryInitialLookbackDays},
		{"BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD", &cfg.BrowserHistoryRepeatVisitThreshold},
		{"BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY", &cfg.BrowserHistoryContentFetchConcurrency},
		{"QF_DECISIONS_PACKET_VERSION", &cfg.QFDecisionsPacketVersion},
		{"QF_DECISIONS_PAGE_SIZE", &cfg.QFDecisionsPageSize},
		{"HOSPITABLE_INITIAL_LOOKBACK_DAYS", &cfg.HospitableInitialLookbackDays},
		{"HOSPITABLE_PAGE_SIZE", &cfg.HospitablePageSize},
		// BUG-020-009 — per-call HTTP client timeouts. Required, no default.
		{"FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS", &cfg.FinancialMarketsHTTPTimeoutSeconds},
		{"AUTH_OAUTH_HTTP_TIMEOUT_SECONDS", &cfg.AuthOAuthHTTPTimeoutSeconds},
	}
	for _, f := range intFields {
		v, err := mustParseIntEnv(f.key)
		if err != nil {
			cfg.intLoadErrs = append(cfg.intLoadErrs, err.Error())
			continue
		}
		*f.dst = v
	}

	// Spec 046 — NATS production hardening. Raw env values are parsed
	// here so the loader can fail fast with a precise error naming the
	// offending key. Validate() then enforces the requiredness contract
	// for the string-form fail-loud check that mirrors the rest of the
	// SST envelope.
	if raw := cfg.NATSMaxReconnectAttemptsRaw; raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("NATS_MAX_RECONNECT_ATTEMPTS: invalid integer %q: %w", raw, err)
		}
		cfg.NATSMaxReconnectAttempts = v
	}
	if raw := cfg.NATSReconnectTimeWaitSecsRaw; raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("NATS_RECONNECT_TIME_WAIT_SECONDS: invalid integer %q: %w", raw, err)
		}
		if v <= 0 {
			return nil, fmt.Errorf("NATS_RECONNECT_TIME_WAIT_SECONDS must be > 0; got %d", v)
		}
		cfg.NATSReconnectTimeWaitSecs = v
	}
	if raw := cfg.NATSMaxPayloadBytesRaw; raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("NATS_MAX_PAYLOAD_BYTES: invalid integer %q: %w", raw, err)
		}
		if v <= 0 {
			return nil, fmt.Errorf("NATS_MAX_PAYLOAD_BYTES must be > 0; got %d", v)
		}
		cfg.NATSMaxPayloadBytes = v
	}
	if raw := cfg.NATSMaxFileStoreBytesRaw; raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("NATS_MAX_FILE_STORE_BYTES: invalid integer %q: %w", raw, err)
		}
		if v <= 0 {
			return nil, fmt.Errorf("NATS_MAX_FILE_STORE_BYTES must be > 0; got %d", v)
		}
		cfg.NATSMaxFileStoreBytes = v
	}
	if raw := cfg.NATSMaxMemStoreBytesRaw; raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("NATS_MAX_MEM_STORE_BYTES: invalid integer %q: %w", raw, err)
		}
		if v <= 0 {
			return nil, fmt.Errorf("NATS_MAX_MEM_STORE_BYTES must be > 0; got %d", v)
		}
		cfg.NATSMaxMemStoreBytes = v
	}
	if raw := cfg.NATSStreamMaxBytesJSON; raw != "" {
		var entries []struct {
			Stream string `json:"stream"`
			Bytes  int64  `json:"bytes"`
		}
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return nil, fmt.Errorf("NATS_STREAM_MAX_BYTES_JSON: invalid JSON: %w", err)
		}
		cfg.NATSStreamMaxBytes = make(map[string]int64, len(entries))
		for _, entry := range entries {
			if entry.Stream == "" {
				return nil, fmt.Errorf("NATS_STREAM_MAX_BYTES_JSON: entry has empty stream name")
			}
			if entry.Bytes <= 0 {
				return nil, fmt.Errorf("NATS_STREAM_MAX_BYTES_JSON: stream %q has non-positive bytes (%d) — every stream MUST have a bounded MaxBytes per spec 046 FR-046-003", entry.Stream, entry.Bytes)
			}
			if _, dup := cfg.NATSStreamMaxBytes[entry.Stream]; dup {
				return nil, fmt.Errorf("NATS_STREAM_MAX_BYTES_JSON: duplicate stream entry %q", entry.Stream)
			}
			cfg.NATSStreamMaxBytes[entry.Stream] = entry.Bytes
		}
	}

	// Spec 048 — Backup and Restore Automation. Integer parsing here so
	// the loader can name the offending env var. requiredVars() catches
	// the missing/empty case with the same fail-loud single-error
	// message pattern as spec 045 / 046.
	if raw := cfg.BackupRetentionDailyRaw; raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("BACKUP_RETENTION_DAILY: invalid integer %q: %w", raw, err)
		}
		if v < 1 {
			return nil, fmt.Errorf("BACKUP_RETENTION_DAILY must be >= 1 (FR-048-001 contract); got %d", v)
		}
		cfg.BackupRetentionDaily = v
	}
	if raw := cfg.BackupRetentionWeeklyRaw; raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("BACKUP_RETENTION_WEEKLY: invalid integer %q: %w", raw, err)
		}
		if v < 0 {
			return nil, fmt.Errorf("BACKUP_RETENTION_WEEKLY must be >= 0; got %d", v)
		}
		cfg.BackupRetentionWeekly = v
	}
	if raw := cfg.BackupWatcherPollSecsRaw; raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("BACKUP_WATCHER_POLL_SECONDS: invalid integer %q: %w", raw, err)
		}
		if v < 1 {
			return nil, fmt.Errorf("BACKUP_WATCHER_POLL_SECONDS must be >= 1; got %d", v)
		}
		cfg.BackupWatcherPollSecs = v
	}

	// Spec 045 — Parse ML_MEMORY_LIMIT (compose-style string like "3G",
	// "512M") into MiB. Empty string leaves MLMemoryLimitMiB=0 so
	// Validate() can name MLMemoryLimit as missing rather than failing
	// here with a less informative parse error.
	if cfg.MLMemoryLimit != "" {
		mib, err := parseComposeMemoryToMiB(cfg.MLMemoryLimit)
		if err != nil {
			return nil, fmt.Errorf("ML_MEMORY_LIMIT: %w", err)
		}
		cfg.MLMemoryLimitMiB = mib
	}

	// BUG-045-001 — Parse OLLAMA_MEMORY_LIMIT (compose-style string
	// like "8G", "1024M") into MiB. Mirrors the ML_MEMORY_LIMIT parse
	// step above byte-for-byte so the ollama bucket of
	// validateModelEnvelopes() can route every configured ollama-
	// resident model against c.OllamaMemoryLimitMiB. Empty string
	// leaves OllamaMemoryLimitMiB=0 so Validate() can name
	// OllamaMemoryLimit as missing via requiredVars() rather than
	// failing here with a less informative parse error.
	if cfg.OllamaMemoryLimit != "" {
		mib, err := parseComposeMemoryToMiB(cfg.OllamaMemoryLimit)
		if err != nil {
			return nil, fmt.Errorf("OLLAMA_MEMORY_LIMIT: %w", err)
		}
		cfg.OllamaMemoryLimitMiB = mib
	}

	// Spec 045 — Parse ML_MODEL_MEMORY_PROFILES_JSON. The generator
	// emits a JSON array of {"model": "...", "memory_mib": N} objects
	// (list-of-objects form because the SST flatten/JSON pipeline cannot
	// safely parse YAML map keys that contain ":"). Convert to the
	// internal map[modelName]MiB representation.
	if rawProfiles := os.Getenv("ML_MODEL_MEMORY_PROFILES_JSON"); rawProfiles != "" {
		var profileList []struct {
			Model     string `json:"model"`
			MemoryMiB int    `json:"memory_mib"`
		}
		if err := json.Unmarshal([]byte(rawProfiles), &profileList); err != nil {
			return nil, fmt.Errorf("ML_MODEL_MEMORY_PROFILES_JSON: invalid JSON: %w", err)
		}
		cfg.MLModelMemoryProfiles = make(map[string]int, len(profileList))
		for _, entry := range profileList {
			if entry.Model == "" {
				return nil, fmt.Errorf("ML_MODEL_MEMORY_PROFILES_JSON: entry has empty model name")
			}
			if entry.MemoryMiB <= 0 {
				return nil, fmt.Errorf("ML_MODEL_MEMORY_PROFILES_JSON: entry %q has non-positive memory_mib (%d)", entry.Model, entry.MemoryMiB)
			}
			cfg.MLModelMemoryProfiles[entry.Model] = entry.MemoryMiB
		}
	}

	// Parse CORS allowed origins (comma-separated)
	if corsOrigins := os.Getenv("CORS_ALLOWED_ORIGINS"); corsOrigins != "" {
		for _, o := range strings.Split(corsOrigins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				cfg.CORSAllowedOrigins = append(cfg.CORSAllowedOrigins, o)
			}
		}
	}

	// Parse runtime trusted-proxy CIDR allowlist (comma-separated CIDRs).
	// BUG-020-005, F-SEC-R30-001 — consumed by internal/api/realip.go to
	// decide whether a given TCP peer is allowed to set X-Forwarded-For /
	// X-Real-IP / True-Client-IP headers that the API will trust.
	if trustedProxies := os.Getenv("RUNTIME_TRUSTED_PROXIES"); trustedProxies != "" {
		for _, p := range strings.Split(trustedProxies, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				cfg.RuntimeTrustedProxies = append(cfg.RuntimeTrustedProxies, p)
			}
		}
	}

	if chatIDs := os.Getenv("TELEGRAM_CHAT_IDS"); chatIDs != "" {
		cfg.TelegramChatIDs = strings.Split(chatIDs, ",")
	}

	// Spec 044 Scope 03 — parse TELEGRAM_USER_MAPPING. The package
	// import for "telegram" is avoided here to keep the dep direction
	// (telegram → config), so the parsing is duplicated as a small
	// helper. The bot calls ParseUserMapping(rawForLogContext) when it
	// also needs the raw form, but for cfg loading we materialize the
	// map directly.
	if rawMapping := os.Getenv("TELEGRAM_USER_MAPPING"); rawMapping != "" {
		parsed, perr := parseTelegramUserMapping(rawMapping)
		if perr != nil {
			return nil, fmt.Errorf("TELEGRAM_USER_MAPPING: %w", perr)
		}
		cfg.TelegramUserMapping = parsed
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	driveCfg, err := loadDriveConfig()
	if err != nil {
		return nil, err
	}
	cfg.Drive = driveCfg

	photosCfg, err := loadPhotosConfig()
	if err != nil {
		return nil, err
	}
	cfg.Photos = photosCfg

	extensionCfg, err := loadExtensionConfig()
	if err != nil {
		return nil, err
	}
	cfg.Extension = extensionCfg

	surfacingCfg, err := loadSurfacingConfig()
	if err != nil {
		return nil, err
	}
	cfg.Surfacing = surfacingCfg

	knowledgeGraphAPICfg, err := loadKnowledgeGraphAPIConfig()
	if err != nil {
		return nil, err
	}
	cfg.KnowledgeGraphAPI = knowledgeGraphAPICfg

	recommendationsCfg, err := loadRecommendationsConfig()
	if err != nil {
		return nil, err
	}
	cfg.Recommendations = recommendationsCfg

	notificationCfg, err := loadNotificationConfig()
	if err != nil {
		return nil, err
	}
	cfg.Notification = notificationCfg

	captureFallbackCfg, err := LoadCaptureFallback()
	if err != nil {
		return nil, err
	}
	cfg.CaptureFallback = captureFallbackCfg

	legacyRetirementCfg, err := LoadLegacyRetirement()
	if err != nil {
		return nil, err
	}
	cfg.LegacyRetirement = legacyRetirementCfg

	// Parse numeric config after string validation passes
	dbMaxConnsStr := os.Getenv("DB_MAX_CONNS")
	dbMinConnsStr := os.Getenv("DB_MIN_CONNS")
	shutdownTimeoutStr := os.Getenv("SHUTDOWN_TIMEOUT_S")
	mlHealthCacheTTLStr := os.Getenv("ML_HEALTH_CACHE_TTL_S")
	mlReadinessTimeoutStr := os.Getenv("ML_READINESS_TIMEOUT_S")

	var parseErrors []string

	if dbMaxConnsStr == "" {
		parseErrors = append(parseErrors, "DB_MAX_CONNS")
	} else if v, err := strconv.ParseInt(dbMaxConnsStr, 10, 32); err != nil || v < 1 {
		parseErrors = append(parseErrors, "DB_MAX_CONNS (must be a positive integer)")
	} else {
		cfg.DBMaxConns = int32(v)
	}

	if dbMinConnsStr == "" {
		parseErrors = append(parseErrors, "DB_MIN_CONNS")
	} else if v, err := strconv.ParseInt(dbMinConnsStr, 10, 32); err != nil || v < 0 {
		parseErrors = append(parseErrors, "DB_MIN_CONNS (must be a non-negative integer)")
	} else {
		cfg.DBMinConns = int32(v)
	}

	if shutdownTimeoutStr == "" {
		parseErrors = append(parseErrors, "SHUTDOWN_TIMEOUT_S")
	} else if v, err := strconv.Atoi(shutdownTimeoutStr); err != nil || v < 1 {
		parseErrors = append(parseErrors, "SHUTDOWN_TIMEOUT_S (must be a positive integer)")
	} else {
		cfg.ShutdownTimeoutS = v
	}

	if mlHealthCacheTTLStr == "" {
		parseErrors = append(parseErrors, "ML_HEALTH_CACHE_TTL_S")
	} else if v, err := strconv.Atoi(mlHealthCacheTTLStr); err != nil || v < 1 {
		parseErrors = append(parseErrors, "ML_HEALTH_CACHE_TTL_S (must be a positive integer)")
	} else {
		cfg.MLHealthCacheTTLS = v
	}

	if mlReadinessTimeoutStr == "" {
		parseErrors = append(parseErrors, "ML_READINESS_TIMEOUT_S")
	} else if v, err := strconv.Atoi(mlReadinessTimeoutStr); err != nil || v < 0 {
		parseErrors = append(parseErrors, "ML_READINESS_TIMEOUT_S (must be a non-negative integer)")
	} else {
		cfg.MLReadinessTimeoutS = v
	}

	if len(parseErrors) > 0 {
		return nil, fmt.Errorf("missing or invalid required configuration: %s", strings.Join(parseErrors, ", "))
	}

	// Cross-validate: DBMinConns must not exceed DBMaxConns
	if cfg.DBMinConns > cfg.DBMaxConns {
		return nil, fmt.Errorf("DB_MIN_CONNS (%d) must not exceed DB_MAX_CONNS (%d)", cfg.DBMinConns, cfg.DBMaxConns)
	}

	// Validate Hospitable config when enabled (SST-compliant — fail-fast for missing values)
	if cfg.HospitableEnabled {
		if cfg.HospitableInitialLookbackDays < 1 {
			return nil, fmt.Errorf("HOSPITABLE_INITIAL_LOOKBACK_DAYS must be a positive integer when Hospitable is enabled")
		}
		if cfg.HospitablePageSize < 1 {
			return nil, fmt.Errorf("HOSPITABLE_PAGE_SIZE must be a positive integer when Hospitable is enabled")
		}
	}

	if cfg.QFDecisionsEnabled {
		if err := cfg.validateQFDecisionsConfig(); err != nil {
			return nil, err
		}
	}

	// Parse optional telegram assembly config (SST-compliant — defaults in smackerel.yaml)
	if v := os.Getenv("TELEGRAM_ASSEMBLY_WINDOW_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 5 && n <= 60 {
			cfg.TelegramAssemblyWindowSeconds = n
		} else {
			return nil, fmt.Errorf("TELEGRAM_ASSEMBLY_WINDOW_SECONDS must be an integer in range [5, 60] (got %q)", v)
		}
	}
	if v := os.Getenv("TELEGRAM_ASSEMBLY_MAX_MESSAGES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 10 && n <= 500 {
			cfg.TelegramAssemblyMaxMessages = n
		} else {
			return nil, fmt.Errorf("TELEGRAM_ASSEMBLY_MAX_MESSAGES must be an integer in range [10, 500] (got %q)", v)
		}
	}
	if v := os.Getenv("TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 2 && n <= 10 {
			cfg.TelegramMediaGroupWindowSeconds = n
		} else {
			return nil, fmt.Errorf("TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS must be an integer in range [2, 10] (got %q)", v)
		}
	}
	if v := os.Getenv("TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 30 && n <= 600 {
			cfg.TelegramDisambiguationTimeoutSeconds = n
		} else {
			return nil, fmt.Errorf("TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS must be an integer in range [30, 600] (got %q)", v)
		}
	}

	// Parse telegram cook session config (SST-compliant — from smackerel.yaml via config generate)
	cookTimeoutStr := os.Getenv("TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES")
	if cookTimeoutStr == "" {
		return nil, fmt.Errorf("TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES is required")
	}
	cookTimeoutVal, err := strconv.Atoi(cookTimeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES: %w", err)
	}
	cfg.TelegramCookSessionTimeoutMinutes = cookTimeoutVal

	cookMaxStr := os.Getenv("TELEGRAM_COOK_SESSION_MAX_PER_CHAT")
	if cookMaxStr == "" {
		return nil, fmt.Errorf("TELEGRAM_COOK_SESSION_MAX_PER_CHAT is required")
	}
	cookMaxVal, err := strconv.Atoi(cookMaxStr)
	if err != nil {
		return nil, fmt.Errorf("invalid TELEGRAM_COOK_SESSION_MAX_PER_CHAT: %w", err)
	}
	cfg.TelegramCookSessionMaxPerChat = cookMaxVal

	// Parse knowledge layer config (SST-compliant — from smackerel.yaml via config generate)
	knowledgeEnabledStr := os.Getenv("KNOWLEDGE_ENABLED")
	if knowledgeEnabledStr == "" {
		return nil, fmt.Errorf("missing required configuration: KNOWLEDGE_ENABLED")
	}
	cfg.KnowledgeEnabled = knowledgeEnabledStr == "true"

	if cfg.KnowledgeEnabled {
		var knowledgeErrors []string

		synthTimeoutStr := os.Getenv("KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS")
		if synthTimeoutStr == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS")
		} else if v, err := strconv.Atoi(synthTimeoutStr); err != nil || v < 1 {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS (must be a positive integer)")
		} else {
			cfg.KnowledgeSynthesisTimeoutSeconds = v
		}

		cfg.KnowledgeLintCron = os.Getenv("KNOWLEDGE_LINT_CRON")
		if cfg.KnowledgeLintCron == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_LINT_CRON")
		} else if !isValidCronExpr(cfg.KnowledgeLintCron) {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_LINT_CRON (not a valid cron expression)")
		}

		staleDaysStr := os.Getenv("KNOWLEDGE_LINT_STALE_DAYS")
		if staleDaysStr == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_LINT_STALE_DAYS")
		} else if v, err := strconv.Atoi(staleDaysStr); err != nil || v < 1 {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_LINT_STALE_DAYS (must be a positive integer)")
		} else {
			cfg.KnowledgeLintStaleDays = v
		}

		maxTokensStr := os.Getenv("KNOWLEDGE_CONCEPT_MAX_TOKENS")
		if maxTokensStr == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_CONCEPT_MAX_TOKENS")
		} else if v, err := strconv.Atoi(maxTokensStr); err != nil || v < 1 {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_CONCEPT_MAX_TOKENS (must be a positive integer)")
		} else {
			cfg.KnowledgeConceptMaxTokens = v
		}

		conceptSearchThresholdStr := os.Getenv("KNOWLEDGE_CONCEPT_SEARCH_THRESHOLD")
		if conceptSearchThresholdStr == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_CONCEPT_SEARCH_THRESHOLD")
		} else if v, err := strconv.ParseFloat(conceptSearchThresholdStr, 64); err != nil || v < 0 || v > 1 {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_CONCEPT_SEARCH_THRESHOLD (must be a float in [0, 1])")
		} else {
			cfg.KnowledgeConceptSearchThreshold = v
		}

		crossSourceThresholdStr := os.Getenv("KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD")
		if crossSourceThresholdStr == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD")
		} else if v, err := strconv.ParseFloat(crossSourceThresholdStr, 64); err != nil || v < 0 || v > 1 {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD (must be a float in [0, 1])")
		} else {
			cfg.KnowledgeCrossSourceConfidenceThreshold = v
		}

		maxRetriesStr := os.Getenv("KNOWLEDGE_MAX_SYNTHESIS_RETRIES")
		if maxRetriesStr == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_MAX_SYNTHESIS_RETRIES")
		} else if v, err := strconv.Atoi(maxRetriesStr); err != nil || v < 0 {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_MAX_SYNTHESIS_RETRIES (must be a non-negative integer)")
		} else {
			cfg.KnowledgeMaxSynthesisRetries = v
		}

		cfg.KnowledgePromptContractIngestSynthesis = os.Getenv("KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS")
		if cfg.KnowledgePromptContractIngestSynthesis == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS")
		}

		cfg.KnowledgePromptContractCrossSource = os.Getenv("KNOWLEDGE_PROMPT_CONTRACT_CROSS_SOURCE")
		if cfg.KnowledgePromptContractCrossSource == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_PROMPT_CONTRACT_CROSS_SOURCE")
		}

		cfg.KnowledgePromptContractLintAudit = os.Getenv("KNOWLEDGE_PROMPT_CONTRACT_LINT_AUDIT")
		if cfg.KnowledgePromptContractLintAudit == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_PROMPT_CONTRACT_LINT_AUDIT")
		}

		cfg.KnowledgePromptContractQueryAugment = os.Getenv("KNOWLEDGE_PROMPT_CONTRACT_QUERY_AUGMENT")
		if cfg.KnowledgePromptContractQueryAugment == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_PROMPT_CONTRACT_QUERY_AUGMENT")
		}

		cfg.KnowledgePromptContractDigestAssembly = os.Getenv("KNOWLEDGE_PROMPT_CONTRACT_DIGEST_ASSEMBLY")
		if cfg.KnowledgePromptContractDigestAssembly == "" {
			knowledgeErrors = append(knowledgeErrors, "KNOWLEDGE_PROMPT_CONTRACT_DIGEST_ASSEMBLY")
		}

		if len(knowledgeErrors) > 0 {
			return nil, fmt.Errorf("missing or invalid required knowledge configuration: %s", strings.Join(knowledgeErrors, ", "))
		}
	}

	// Parse prompt contracts dir (SST-compliant — from smackerel.yaml via config generate)
	cfg.PromptContractsDir = os.Getenv("PROMPT_CONTRACTS_DIR")

	// Parse observability config (SST-compliant — opt-in, disabled by default)
	cfg.OTELEnabled = os.Getenv("OTEL_ENABLED") == "true"
	cfg.OTELExporterEndpoint = os.Getenv("OTEL_EXPORTER_ENDPOINT")

	// Parse expense tracking config (SST-compliant — from smackerel.yaml via config generate)
	expensesEnabledStr := os.Getenv("EXPENSES_ENABLED")
	if expensesEnabledStr == "" {
		return nil, fmt.Errorf("missing required configuration: EXPENSES_ENABLED")
	}
	cfg.ExpensesEnabled = expensesEnabledStr == "true"

	if cfg.ExpensesEnabled {
		var expenseErrors []string

		cfg.ExpensesDefaultCurrency = os.Getenv("EXPENSES_DEFAULT_CURRENCY")
		if cfg.ExpensesDefaultCurrency == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_DEFAULT_CURRENCY")
		}

		exportMaxRowsStr := os.Getenv("EXPENSES_EXPORT_MAX_ROWS")
		if exportMaxRowsStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_EXPORT_MAX_ROWS")
		} else if v, err := strconv.Atoi(exportMaxRowsStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_EXPORT_MAX_ROWS (must be a positive integer)")
		} else {
			cfg.ExpensesExportMaxRows = v
		}

		cfg.ExpensesExportQBDateFormat = os.Getenv("EXPENSES_EXPORT_QB_DATE_FORMAT")
		if cfg.ExpensesExportQBDateFormat == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_EXPORT_QB_DATE_FORMAT")
		}

		cfg.ExpensesExportStdDateFormat = os.Getenv("EXPENSES_EXPORT_STD_DATE_FORMAT")
		if cfg.ExpensesExportStdDateFormat == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_EXPORT_STD_DATE_FORMAT")
		}

		minConfStr := os.Getenv("EXPENSES_SUGGESTIONS_MIN_CONFIDENCE")
		if minConfStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_MIN_CONFIDENCE")
		} else if v, err := strconv.ParseFloat(minConfStr, 64); err != nil || v < 0 || v > 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_MIN_CONFIDENCE (must be a float in [0, 1])")
		} else {
			cfg.ExpensesSuggestionsMinConfidence = v
		}

		minPastStr := os.Getenv("EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS")
		if minPastStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS")
		} else if v, err := strconv.Atoi(minPastStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS (must be a positive integer)")
		} else {
			cfg.ExpensesSuggestionsMinPastBusiness = v
		}

		maxPerDigestStr := os.Getenv("EXPENSES_SUGGESTIONS_MAX_PER_DIGEST")
		if maxPerDigestStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_MAX_PER_DIGEST")
		} else if v, err := strconv.Atoi(maxPerDigestStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_MAX_PER_DIGEST (must be a positive integer)")
		} else {
			cfg.ExpensesSuggestionsMaxPerDigest = v
		}

		reclassLimStr := os.Getenv("EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT")
		if reclassLimStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT")
		} else if v, err := strconv.Atoi(reclassLimStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT (must be a positive integer)")
		} else {
			cfg.ExpensesSuggestionsReclassifyBatchLim = v
		}

		vendorCacheStr := os.Getenv("EXPENSES_VENDOR_CACHE_SIZE")
		if vendorCacheStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_VENDOR_CACHE_SIZE")
		} else if v, err := strconv.Atoi(vendorCacheStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_VENDOR_CACHE_SIZE (must be a positive integer)")
		} else {
			cfg.ExpensesVendorCacheSize = v
		}

		digestMaxWordsStr := os.Getenv("EXPENSES_DIGEST_MAX_WORDS")
		if digestMaxWordsStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_DIGEST_MAX_WORDS")
		} else if v, err := strconv.Atoi(digestMaxWordsStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_DIGEST_MAX_WORDS (must be a positive integer)")
		} else {
			cfg.ExpensesDigestMaxWords = v
		}

		digestNeedsReviewStr := os.Getenv("EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT")
		if digestNeedsReviewStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT")
		} else if v, err := strconv.Atoi(digestNeedsReviewStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT (must be a positive integer)")
		} else {
			cfg.ExpensesDigestNeedsReviewLimit = v
		}

		missingReceiptStr := os.Getenv("EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS")
		if missingReceiptStr == "" {
			expenseErrors = append(expenseErrors, "EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS")
		} else if v, err := strconv.Atoi(missingReceiptStr); err != nil || v < 1 {
			expenseErrors = append(expenseErrors, "EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS (must be a positive integer)")
		} else {
			cfg.ExpensesDigestMissingReceiptLookback = v
		}

		// JSON-encoded complex config
		imapLabelsStr := os.Getenv("IMAP_EXPENSE_LABELS")
		if imapLabelsStr == "" || imapLabelsStr == "{}" {
			cfg.IMAPExpenseLabels = make(map[string]string)
		} else if err := json.Unmarshal([]byte(imapLabelsStr), &cfg.IMAPExpenseLabels); err != nil {
			expenseErrors = append(expenseErrors, "IMAP_EXPENSE_LABELS (invalid JSON)")
		}

		businessVendorsStr := os.Getenv("EXPENSES_BUSINESS_VENDORS")
		if businessVendorsStr == "" || businessVendorsStr == "[]" {
			cfg.ExpensesBusinessVendors = []string{}
		} else if err := json.Unmarshal([]byte(businessVendorsStr), &cfg.ExpensesBusinessVendors); err != nil {
			expenseErrors = append(expenseErrors, "EXPENSES_BUSINESS_VENDORS (invalid JSON)")
		}

		categoriesStr := os.Getenv("EXPENSES_CATEGORIES")
		if categoriesStr == "" || categoriesStr == "[]" {
			expenseErrors = append(expenseErrors, "EXPENSES_CATEGORIES (must contain at least one category)")
		} else if err := json.Unmarshal([]byte(categoriesStr), &cfg.ExpensesCategories); err != nil {
			expenseErrors = append(expenseErrors, "EXPENSES_CATEGORIES (invalid JSON)")
		}

		if len(expenseErrors) > 0 {
			return nil, fmt.Errorf("missing or invalid required expense configuration: %s", strings.Join(expenseErrors, ", "))
		}
	}

	// Parse meal planning config (SST-compliant — from smackerel.yaml via config generate)
	mealPlanEnabledStr := os.Getenv("MEAL_PLANNING_ENABLED")
	if mealPlanEnabledStr == "" {
		return nil, fmt.Errorf("missing required configuration: MEAL_PLANNING_ENABLED")
	}
	cfg.MealPlanEnabled = mealPlanEnabledStr == "true"

	if cfg.MealPlanEnabled {
		var mealPlanErrors []string

		defaultServStr := os.Getenv("MEAL_PLANNING_DEFAULT_SERVINGS")
		if defaultServStr == "" {
			mealPlanErrors = append(mealPlanErrors, "MEAL_PLANNING_DEFAULT_SERVINGS")
		} else if v, err := strconv.Atoi(defaultServStr); err != nil || v < 1 {
			mealPlanErrors = append(mealPlanErrors, "MEAL_PLANNING_DEFAULT_SERVINGS (must be a positive integer)")
		} else {
			cfg.MealPlanDefaultServings = v
		}

		mealTypesStr := os.Getenv("MEAL_PLANNING_MEAL_TYPES")
		if mealTypesStr == "" {
			mealPlanErrors = append(mealPlanErrors, "MEAL_PLANNING_MEAL_TYPES")
		} else {
			// Parse comma-separated, stripping brackets and quotes from YAML array format
			cleaned := strings.Trim(mealTypesStr, "[] ")
			var types []string
			for _, t := range strings.Split(cleaned, ",") {
				t = strings.Trim(strings.TrimSpace(t), "\"'")
				if t != "" {
					types = append(types, t)
				}
			}
			if len(types) == 0 {
				mealPlanErrors = append(mealPlanErrors, "MEAL_PLANNING_MEAL_TYPES (must contain at least one type)")
			} else {
				cfg.MealPlanMealTypes = types
			}
		}

		cfg.MealPlanMealTimes = make(map[string]string)
		mealTimeKeys := map[string]string{
			"MEAL_PLANNING_MEAL_TIME_BREAKFAST": "breakfast",
			"MEAL_PLANNING_MEAL_TIME_LUNCH":     "lunch",
			"MEAL_PLANNING_MEAL_TIME_DINNER":    "dinner",
			"MEAL_PLANNING_MEAL_TIME_SNACK":     "snack",
		}
		for envKey, mealKey := range mealTimeKeys {
			if v := os.Getenv(envKey); v != "" {
				cfg.MealPlanMealTimes[mealKey] = v
			}
		}

		cfg.MealPlanCalendarSync = os.Getenv("MEAL_PLANNING_CALENDAR_SYNC") == "true"

		cfg.MealPlanAutoComplete = os.Getenv("MEAL_PLANNING_AUTO_COMPLETE") == "true"

		cfg.MealPlanAutoCompleteCron = os.Getenv("MEAL_PLANNING_AUTO_COMPLETE_CRON")
		if cfg.MealPlanAutoComplete {
			if cfg.MealPlanAutoCompleteCron == "" {
				mealPlanErrors = append(mealPlanErrors, "MEAL_PLANNING_AUTO_COMPLETE_CRON")
			} else if !isValidCronExpr(cfg.MealPlanAutoCompleteCron) {
				mealPlanErrors = append(mealPlanErrors, "MEAL_PLANNING_AUTO_COMPLETE_CRON (not a valid cron expression)")
			}
		}

		if len(mealPlanErrors) > 0 {
			return nil, fmt.Errorf("missing or invalid required meal planning configuration: %s", strings.Join(mealPlanErrors, ", "))
		}
	}

	// Parse card rewards config (Spec 083 — SST-compliant, from smackerel.yaml
	// via config generate). When CARD_REWARDS_ENABLED != "true" this returns a
	// disabled config with no error; when enabled it fails loud naming any
	// missing/invalid required key (SCN-083-A03, A04).
	cardRewardsCfg, err := LoadCardRewardsConfig()
	if err != nil {
		return nil, err
	}
	cfg.CardRewards = cardRewardsCfg

	// Spec 044 — Per-user bearer auth foundation. Every AUTH_* key is
	// REQUIRED at the SST generator boundary; secret-bearing fields are
	// allowed to be empty in dev/test (Auth.Enabled=false) and validated
	// here for production-mode (Environment=="production" + Enabled=true).
	if err := loadAuthConfig(cfg); err != nil {
		return nil, err
	}

	// Spec 061 SCOPE-01 — Conversational Assistant SST envelope.
	// Fails loud with [F061-SST-MISSING] if any ASSISTANT_* key is
	// missing or unparseable.
	if err := loadAssistantConfig(cfg); err != nil {
		return nil, err
	}

	// Spec 061 SCOPE-05 design §7.2 rules #2-#10 — assistant config
	// validation runs AFTER loadAssistantConfig populates the struct.
	// The earlier cfg.Validate() at the end of the env-load block
	// short-circuited when Enabled was still false; re-run the
	// assistant-specific portion here so that webhook-mode secret
	// resolution (TelegramWebhookSecret = os.Getenv(SecretRef))
	// actually populates before main() reads it.
	if err := cfg.validateAssistantConfig(); err != nil {
		return nil, err
	}

	// Spec 064 SCOPE-03 — open-ended knowledge agent SST. Loaded
	// after the spec 061 assistant block since it logically sits on
	// AssistantConfig.OpenKnowledge. Fail-loud on missing/invalid
	// env vars; deep validation gated on Enabled per design §SST.
	ok, err := LoadOpenKnowledge()
	if err != nil {
		return nil, err
	}
	cfg.Assistant.OpenKnowledge = ok

	// Spec 096 SCOPE-01 — Multi-Provider Model Connection registry. Loaded
	// after the open-knowledge block since the registry sits on the same
	// llm.* SST surface the agent consumes. Fail-loud closed-set Validate()
	// runs inside LoadModelConnections (against the canonical SecretKeys()
	// manifest); a missing env var, unknown kind, missing per-kind param,
	// non-positive discovery bound, undeclared env-mode secret, or an
	// enabled non-ollama model with no model_costs entry aborts here.
	mc, err := LoadModelConnections()
	if err != nil {
		return nil, err
	}
	cfg.ModelConnections = mc

	// Spec 095 SCOPE-01 — Retrieval-Strategy Routing + Freshness-Aware
	// Retrieval SST. Loaded after the assistant block since the router
	// consumes the assistant intent substrate at runtime. Fail-loud
	// [F095-SST-MISSING] on any missing/invalid RETRIEVAL_* key;
	// validation is unconditional (no Enabled short-circuit) per design §10.
	retrievalCfg, err := LoadRetrieval()
	if err != nil {
		return nil, err
	}
	cfg.Retrieval = retrievalCfg

	return cfg, nil
}

// loadAuthConfig populates cfg.Auth from AUTH_* env vars and validates
// production-mode invariants. Spec 044 SCN-AUTH-005..006/011 — empty
// secret material is accepted in dev/test, fail-loud in production with
// auth.enabled=true.
func loadAuthConfig(cfg *Config) error {
	cfg.Auth.TokenFormat = os.Getenv("AUTH_TOKEN_FORMAT")
	cfg.Auth.SigningActivePrivateKey = os.Getenv("AUTH_SIGNING_ACTIVE_PRIVATE_KEY")
	cfg.Auth.SigningActiveKeyID = os.Getenv("AUTH_SIGNING_ACTIVE_KEY_ID")
	cfg.Auth.SigningPriorPublicKey = os.Getenv("AUTH_SIGNING_PRIOR_PUBLIC_KEY")
	cfg.Auth.SigningPriorKeyID = os.Getenv("AUTH_SIGNING_PRIOR_KEY_ID")
	cfg.Auth.RevocationNATSSubject = os.Getenv("AUTH_REVOCATION_NATS_SUBJECT")
	cfg.Auth.AtRestHashingKey = os.Getenv("AUTH_AT_REST_HASHING_KEY")
	cfg.Auth.TelemetryMetricPrefix = os.Getenv("AUTH_TELEMETRY_METRIC_PREFIX")
	cfg.Auth.BootstrapToken = os.Getenv("AUTH_BOOTSTRAP_TOKEN")
	// Spec 091 — OPTIONAL web self-registration invite token. Empty ⇒
	// registration disabled at POST (never open signup). Deliberately NOT
	// appended to authErrors below: unlike AUTH_BOOTSTRAP_TOKEN it is not
	// production-required, so an empty value must never fail boot.
	cfg.Auth.WebRegistrationInviteToken = os.Getenv("WEB_REGISTRATION_INVITE_TOKEN")

	var authErrors []string

	if v := os.Getenv("AUTH_ENABLED"); v == "" {
		authErrors = append(authErrors, "AUTH_ENABLED")
	} else {
		cfg.Auth.Enabled = v == "true"
	}

	if cfg.Auth.TokenFormat == "" {
		authErrors = append(authErrors, "AUTH_TOKEN_FORMAT")
	} else if cfg.Auth.TokenFormat != "paseto-v4-public" {
		authErrors = append(authErrors, "AUTH_TOKEN_FORMAT (must be \"paseto-v4-public\" — spec 044 OQ-1)")
	}

	if v := os.Getenv("AUTH_TOKEN_TTL_HOURS"); v == "" {
		authErrors = append(authErrors, "AUTH_TOKEN_TTL_HOURS")
	} else if n, err := strconv.Atoi(v); err != nil || n < 1 {
		authErrors = append(authErrors, "AUTH_TOKEN_TTL_HOURS (must be a positive integer)")
	} else {
		cfg.Auth.TokenTTLHours = n
	}

	if v := os.Getenv("AUTH_ROTATION_GRACE_WINDOW_HOURS"); v == "" {
		authErrors = append(authErrors, "AUTH_ROTATION_GRACE_WINDOW_HOURS")
	} else if n, err := strconv.Atoi(v); err != nil || n < 24 {
		authErrors = append(authErrors, "AUTH_ROTATION_GRACE_WINDOW_HOURS (must be ≥ 24 — NFR-AUTH-003)")
	} else {
		cfg.Auth.RotationGraceWindowHours = n
	}

	if v := os.Getenv("AUTH_CLOCK_SKEW_TOLERANCE_SECONDS"); v == "" {
		authErrors = append(authErrors, "AUTH_CLOCK_SKEW_TOLERANCE_SECONDS")
	} else if n, err := strconv.Atoi(v); err != nil || n < 0 || n > 60 {
		authErrors = append(authErrors, "AUTH_CLOCK_SKEW_TOLERANCE_SECONDS (must be in range [0, 60] — NFR-AUTH-005)")
	} else {
		cfg.Auth.ClockSkewToleranceSeconds = n
	}

	if v := os.Getenv("AUTH_REVOCATION_CACHE_REFRESH_INTERVAL_SECONDS"); v == "" {
		authErrors = append(authErrors, "AUTH_REVOCATION_CACHE_REFRESH_INTERVAL_SECONDS")
	} else if n, err := strconv.Atoi(v); err != nil || n < 1 {
		authErrors = append(authErrors, "AUTH_REVOCATION_CACHE_REFRESH_INTERVAL_SECONDS (must be a positive integer)")
	} else {
		cfg.Auth.RevocationCacheRefreshIntervalSeconds = n
	}

	if cfg.Auth.RevocationNATSSubject == "" {
		authErrors = append(authErrors, "AUTH_REVOCATION_NATS_SUBJECT")
	}

	if v := os.Getenv("AUTH_PRODUCTION_SHARED_TOKEN_FALLBACK_ENABLED"); v == "" {
		authErrors = append(authErrors, "AUTH_PRODUCTION_SHARED_TOKEN_FALLBACK_ENABLED")
	} else {
		cfg.Auth.ProductionSharedTokenFallbackEnabled = v == "true"
	}

	if v := os.Getenv("AUTH_TELEMETRY_ENABLED"); v == "" {
		authErrors = append(authErrors, "AUTH_TELEMETRY_ENABLED")
	} else {
		cfg.Auth.TelemetryEnabled = v == "true"
	}

	if cfg.Auth.TelemetryMetricPrefix == "" {
		authErrors = append(authErrors, "AUTH_TELEMETRY_METRIC_PREFIX")
	}

	// Production-mode validation — reject empty secret material when
	// auth.enabled is true and SMACKEREL_ENV is "production". Dev/test
	// configurations preserve the SMACKEREL_AUTH_TOKEN ergonomic
	// (SCN-AUTH-005/011).
	if cfg.Environment == "production" && cfg.Auth.Enabled {
		if cfg.Auth.SigningActivePrivateKey == "" {
			authErrors = append(authErrors, "AUTH_SIGNING_ACTIVE_PRIVATE_KEY (REQUIRED in production with auth.enabled=true)")
		}
		if cfg.Auth.SigningActiveKeyID == "" {
			authErrors = append(authErrors, "AUTH_SIGNING_ACTIVE_KEY_ID (REQUIRED in production with auth.enabled=true)")
		}
		if cfg.Auth.AtRestHashingKey == "" {
			authErrors = append(authErrors, "AUTH_AT_REST_HASHING_KEY (REQUIRED in production with auth.enabled=true)")
		}
		// Spec 051 FR-051-004 / SCN-051-S01 — bootstrap token is required
		// at config-load time when running in production with auth enabled.
		// The wiring-time check in internal/auth/startup.go remains as
		// defense-in-depth; this loader-time gate refuses to even produce
		// a Config object that would need to be repaired later.
		if cfg.Auth.BootstrapToken == "" {
			authErrors = append(authErrors, "AUTH_BOOTSTRAP_TOKEN (REQUIRED in production with auth.enabled=true — spec 051 FR-051-004)")
		}
		// At-rest hashing key MUST differ from the signing key (OQ-8).
		if cfg.Auth.AtRestHashingKey != "" && cfg.Auth.SigningActivePrivateKey != "" &&
			cfg.Auth.AtRestHashingKey == cfg.Auth.SigningActivePrivateKey {
			authErrors = append(authErrors, "AUTH_AT_REST_HASHING_KEY (MUST differ from AUTH_SIGNING_ACTIVE_PRIVATE_KEY — spec 044 OQ-8)")
		}
		// If a prior key id is set, the prior public key MUST also be set
		// (and vice versa) — partial rotation state is a configuration bug.
		if (cfg.Auth.SigningPriorPublicKey == "") != (cfg.Auth.SigningPriorKeyID == "") {
			authErrors = append(authErrors, "AUTH_SIGNING_PRIOR_PUBLIC_KEY and AUTH_SIGNING_PRIOR_KEY_ID (both must be set together or both empty)")
		}
	}

	if len(authErrors) > 0 {
		return fmt.Errorf("missing or invalid required auth configuration: %s", strings.Join(authErrors, ", "))
	}
	return nil
}

// requiredVars returns the list of required environment variable names
// and their corresponding values from the config.
func (c *Config) requiredVars() []struct {
	Name  string
	Value string
} {
	vars := []struct {
		Name  string
		Value string
	}{
		{"DATABASE_URL", c.DatabaseURL},
		{"NATS_URL", c.NATSURL},
		{"LLM_PROVIDER", c.LLMProvider},
		{"LLM_MODEL", c.LLMModel},
		{"SMACKEREL_ENV", c.Environment},
		{"EMBEDDING_MODEL", c.EmbeddingModel},
		{"DIGEST_CRON", c.DigestCron},
		{"LOG_LEVEL", c.LogLevel},
		{"PORT", c.Port},
		{"ML_SIDECAR_URL", c.MLSidecarURL},
		{"CORE_API_URL", c.CoreAPIURL},
		// Spec 045 FR-045-001 / FR-045-002 — deploy resource envelope is
		// required at every load. Missing values fail-loud here so the
		// operator gets one error message naming all missing env vars.
		{"POSTGRES_CPU_LIMIT", c.PostgresCPULimit},
		{"POSTGRES_MEMORY_LIMIT", c.PostgresMemoryLimit},
		{"NATS_CPU_LIMIT", c.NATSCPULimit},
		{"NATS_MEMORY_LIMIT", c.NATSMemoryLimit},
		{"CORE_CPU_LIMIT", c.CoreCPULimit},
		{"CORE_MEMORY_LIMIT", c.CoreMemoryLimit},
		{"ML_CPU_LIMIT", c.MLCPULimit},
		{"ML_MEMORY_LIMIT", c.MLMemoryLimit},
		{"OLLAMA_CPU_LIMIT", c.OllamaCPULimit},
		{"OLLAMA_MEMORY_LIMIT", c.OllamaMemoryLimit},
		// BUG-045-001 — Per-service envelope routing requires the
		// SST emission of every ollama-routed and ml-sidecar-routed
		// model env var. Names below MUST match the emitted set in
		// scripts/commands/config.sh (12 ollama + 2 ml-sidecar today).
		// OLLAMA_OCR_MODEL, OLLAMA_REASONING_MODEL, and
		// OLLAMA_FAST_MODEL are forward-compatible Config struct
		// fields but are NOT yet emitted by config.sh so they are
		// NOT named here. Once config.sh starts emitting them the
		// entries below MUST be extended in the same PR.
		{"OLLAMA_VISION_MODEL", c.OllamaVisionModel},
		{"PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", c.PhotosIntelligenceClassifyModel},
		{"PHOTOS_INTELLIGENCE_EMBED_MODEL", c.PhotosIntelligenceEmbedModel},
		{"PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL", c.PhotosIntelligenceSensitivityModel},
		{"PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", c.PhotosIntelligenceAestheticModel},
		{"PHOTOS_INTELLIGENCE_OCR_MODEL", c.PhotosIntelligenceOcrModel},
		{"AGENT_PROVIDER_DEFAULT_MODEL", c.AgentProviderDefaultModel},
		{"AGENT_PROVIDER_REASONING_MODEL", c.AgentProviderReasoningModel},
		{"AGENT_PROVIDER_FAST_MODEL", c.AgentProviderFastModel},
		{"AGENT_PROVIDER_VISION_MODEL", c.AgentProviderVisionModel},
		{"AGENT_PROVIDER_OCR_MODEL", c.AgentProviderOcrModel},
		// Spec 046 FR-046-001 / FR-046-002 / FR-046-003 — NATS production
		// hardening envelope. Bytes/integer parsing is done in Load();
		// missing string-form values are caught here with the same
		// fail-loud single-error-message pattern as spec 045 above.
		{"NATS_MAX_RECONNECT_ATTEMPTS", c.NATSMaxReconnectAttemptsRaw},
		{"NATS_RECONNECT_TIME_WAIT_SECONDS", c.NATSReconnectTimeWaitSecsRaw},
		{"NATS_MAX_PAYLOAD_BYTES", c.NATSMaxPayloadBytesRaw},
		{"NATS_MAX_FILE_STORE_BYTES", c.NATSMaxFileStoreBytesRaw},
		{"NATS_MAX_MEM_STORE_BYTES", c.NATSMaxMemStoreBytesRaw},
		{"NATS_STREAM_MAX_BYTES_JSON", c.NATSStreamMaxBytesJSON},
		// Spec 081 FR-081-001 / FR-081-002 — Python sidecar consumer
		// contract. Consumed by ml/app/nats_client.py; required at
		// boot so the env file emits them without drift.
		{"NATS_CONSUMER_MAX_DELIVER", c.NATSConsumerMaxDeliverRaw},
		{"NATS_CONSUMER_ACK_WAIT_SECONDS", c.NATSConsumerAckWaitSecondsRaw},
		// Spec 048 FR-048-001 / FR-048-002 — backup and restore
		// automation envelope. Every key is required; missing values
		// fail-loud here with the rest of the envelope. Retention
		// integers are validated for value ranges in Load() above.
		{"BACKUP_LOCAL_DIR", c.BackupLocalDir},
		{"BACKUP_STATUS_FILE", c.BackupStatusFile},
		{"BACKUP_RETENTION_DAILY", c.BackupRetentionDailyRaw},
		{"BACKUP_RETENTION_WEEKLY", c.BackupRetentionWeeklyRaw},
		{"BACKUP_WATCHER_POLL_SECONDS", c.BackupWatcherPollSecsRaw},
		{"NTFY_SOURCES_JSON", c.NtfySourcesJSON},
	}
	// MIT-040-S-004 — SMACKEREL_AUTH_TOKEN is NOT in requiredVars(): it is
	// required only when SMACKEREL_ENV=production, and the dedicated
	// production-mode check in Validate() emits a single error that names
	// both the production constraint and the variable name. In
	// development/test, an empty token is allowed at the loader layer; the
	// runtime warn-and-continue is logged by configureLogging() and the
	// bearer middleware enforces 401 in production as defense-in-depth.
	// Ollama vars are only required when using Ollama as the LLM provider
	if strings.EqualFold(c.LLMProvider, "ollama") {
		vars = append(vars,
			struct{ Name, Value string }{"OLLAMA_URL", c.OllamaURL},
			struct{ Name, Value string }{"OLLAMA_MODEL", c.OllamaModel},
		)
	}
	return vars
}

// Validate checks that all required configuration values are present.
// Returns an error listing all missing variables.
func (c *Config) Validate() error {
	var missing []string
	for _, v := range c.requiredVars() {
		if v.Value == "" {
			missing = append(missing, v.Name)
		}
	}
	// LLM_API_KEY is required unless using Ollama
	if !strings.EqualFold(c.LLMProvider, "ollama") && c.LLMAPIKey == "" {
		missing = append(missing, "LLM_API_KEY")
	}
	// BUG-020-008 — fold fail-loud int parse errors (missing or
	// unparseable values for the 8 SST-required int env vars) into the
	// same consolidated missing-keys error so a single boot surfaces
	// every offender at once.
	missing = append(missing, c.intLoadErrs...)
	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}

	// BUG-020-009 — fail-loud range guards for the two new HTTP timeout
	// SST keys. Zero or negative values are meaningless / Go runtime
	// traps; reject explicitly with a message naming the offending key.
	if c.FinancialMarketsHTTPTimeoutSeconds <= 0 {
		return fmt.Errorf("FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS must be > 0; got %d", c.FinancialMarketsHTTPTimeoutSeconds)
	}
	if c.AuthOAuthHTTPTimeoutSeconds <= 0 {
		return fmt.Errorf("AUTH_OAUTH_HTTP_TIMEOUT_SECONDS must be > 0; got %d", c.AuthOAuthHTTPTimeoutSeconds)
	}

	// MIT-040-S-004 — enforce SMACKEREL_ENV allowlist (development | test |
	// production). Any other value is a configuration error.
	switch c.Environment {
	case "development", "test", "production":
		// allowed
	default:
		return fmt.Errorf("SMACKEREL_ENV must be one of development|test|production, got %q", c.Environment)
	}

	// MIT-040-S-004 — explicit production-mode auth-token error. The
	// missing-required path above already names SMACKEREL_AUTH_TOKEN when
	// the env is production, but the next message bundles the production
	// constraint with the variable name so operators see both terms in a
	// single error and tests can assert the production mode by message.
	if c.Environment == "production" && c.AuthToken == "" {
		return fmt.Errorf("SMACKEREL_AUTH_TOKEN must be set when SMACKEREL_ENV=production")
	}

	// Spec 051 FR-051-005 / SCN-051-S02 — defense-in-depth: even if the
	// SST loader misses the dev-default Postgres password, refuse it at
	// runtime when SMACKEREL_ENV=production. The error names the env var
	// without echoing the value (FR-051-007 redaction contract).
	if c.Environment == "production" {
		dbPassword := extractDatabasePassword(c.DatabaseURL)
		if IsDevDBPassword(dbPassword) {
			return fmt.Errorf("DATABASE_URL password component is set to a known dev-default value — generate a strong random Postgres password (POSTGRES_PASSWORD) before deploying with SMACKEREL_ENV=production (spec 051 FR-051-005)")
		}
	}

	// Spec 052 FR-052-007 — defense-in-depth: refuse to start if any
	// SST-managed secret key has reached the runtime still equal to its
	// placeholder marker. The placeholder marker is emitted by the SST
	// loader for production-class targets (see internal/config/secret_keys.go
	// + scripts/commands/config.sh) and MUST be substituted by the deploy
	// adapter (knb) before the bundle env-files are loaded into the
	// container. If a placeholder reaches Validate(), the adapter
	// substitution failed (or was skipped) — the runtime MUST refuse to
	// start to avoid a process that thinks it has secrets but actually
	// has the marker string. The error names the offending KEY only —
	// never the placeholder marker literal and never the resolved value
	// (FR-051-007 redaction contract extended). Fires unconditionally
	// (defense-in-depth across every environment) because a placeholder
	// is never a legitimate secret value in any environment per
	// FR-052-011 (dev/test bundles ship literals; production-class
	// bundles ship placeholders that the adapter substitutes). For each
	// declared key in SecretKeys(), read its resolved value from the
	// matching authoritative source — POSTGRES_PASSWORD comes from the
	// DATABASE_URL credential component (already parsed into c.DatabaseURL
	// by Load()); AUTH_* keys come straight from os.Getenv because
	// loadAuthConfig() runs AFTER Validate() in Load() so c.Auth.*
	// fields are still empty at this point. Reading from os.Getenv for
	// AUTH_* mirrors the env-var pipeline that the deploy adapter
	// substitution targets and is consistent with how the existing
	// FR-051-005 dev-default check operates against DATABASE_URL.
	for _, key := range SecretKeys() {
		var value string
		switch key {
		case "POSTGRES_PASSWORD":
			value = extractDatabasePassword(c.DatabaseURL)
		default:
			// AUTH_SIGNING_ACTIVE_PRIVATE_KEY, AUTH_AT_REST_HASHING_KEY,
			// AUTH_BOOTSTRAP_TOKEN, plus any future managed secret key
			// not mapped into a Config struct field at Validate() time.
			value = os.Getenv(key)
		}
		if IsPlaceholder(value) {
			return fmt.Errorf("%s still equals placeholder marker — adapter substitution failed (spec 052 FR-052-007)", key)
		}
	}

	// AUTH_TOKEN format checks only apply when a token is set. In
	// development/test with an empty token, the dev-mode bypass governs.
	if c.AuthToken != "" {
		// Reject known placeholder auth tokens — these are guessable defaults
		placeholders := []string{
			"development-change-me",
			"changeme",
			"change-me",
			"placeholder",
			"test-token",
			"default",
			"dev-token-smackerel-2026",
		}
		for _, p := range placeholders {
			if strings.EqualFold(c.AuthToken, p) {
				// FR-051-007 redaction contract: name the offending KEY but
				// never echo the offending VALUE (no %q on c.AuthToken).
				return fmt.Errorf("SMACKEREL_AUTH_TOKEN is set to a known placeholder value — generate a secure random token: openssl rand -hex 24")
			}
		}
		// Reject any token starting with "dev-token-" — these are development-only patterns
		if strings.HasPrefix(strings.ToLower(c.AuthToken), "dev-token-") {
			return fmt.Errorf("SMACKEREL_AUTH_TOKEN starts with 'dev-token-' which is a development placeholder pattern — generate a secure random token: openssl rand -hex 24")
		}
		if len(c.AuthToken) < 16 {
			return fmt.Errorf("SMACKEREL_AUTH_TOKEN must be at least 16 characters (got %d)", len(c.AuthToken))
		}
	}

	// Semantic validation: PORT must be a valid TCP port number
	if c.Port != "" {
		port, err := strconv.Atoi(c.Port)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("PORT must be a number between 1 and 65535 (got %q)", c.Port)
		}
	}

	// Semantic validation: LOG_LEVEL must be a recognized value
	if c.LogLevel != "" {
		switch strings.ToLower(c.LogLevel) {
		case "debug", "info", "warn", "error":
			// valid
		default:
			return fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error (got %q)", c.LogLevel)
		}
	}

	// Semantic validation: DIGEST_CRON must look like a valid 5-field cron expression
	if c.DigestCron != "" {
		if !isValidCronExpr(c.DigestCron) {
			return fmt.Errorf("DIGEST_CRON is not a valid cron expression (got %q)", c.DigestCron)
		}
	}

	// Spec 061 Round 66 (2026-05-30) — operator quality directive reversed
	// the Round 64 SCOPE-06a test-env model override requirement. Test env
	// now inherits the base `gemma3:4b` defaults (background latency
	// acceptable; retrieval-qa-v1 timeout raised to 60s). The
	// `validateTestEnvModelOverrides` validator and its test file have been
	// deleted as obsolete. No replacement validator is required: base SST
	// keys are still required (no fallbacks); test env simply does not
	// override them.

	// Spec 045 FR-045-002 — Per-service model envelope check (BUG-045-001).
	// validateModelEnvelopes routes each configured model env var into
	// either the ollama bucket (checked against c.OllamaMemoryLimitMiB)
	// or the ml-sidecar bucket (checked against c.MLMemoryLimitMiB).
	// MLMemoryLimitMiB, OllamaMemoryLimitMiB, and MLModelMemoryProfiles
	// are all surfaced by the requiredVars() check above when their
	// SST env vars are missing. With the envelopes known, reject any
	// configured model whose profile exceeds its bucket's envelope OR
	// whose profile entry is missing entirely. The fail-loud error
	// names every offender in one message so the operator can fix all
	// problems in one pass.
	if err := c.validateModelEnvelopes(); err != nil {
		return err
	}

	// Spec 061 SCOPE-01 — Conversational Assistant rule-based
	// validation (design §7.2 rules #2–#4). Rule #1 (required values)
	// is enforced inline by loadAssistantConfig at Load() time with
	// the [F061-SST-MISSING] prefix. The rule validation itself is
	// invoked explicitly from Load() AFTER loadAssistantConfig has
	// populated the Assistant struct (and AFTER the webhook secret
	// indirection has resolved); it is intentionally NOT re-invoked
	// here because Validate() is also called EARLIER in Load() to
	// validate base config, at which point Assistant.Observability
	// would still be zero-valued and the OTel rules would false-fire.
	// External callers that need full validation should call Load()
	// — it runs validateAssistantConfig as part of its post-load
	// sequence. See observability_test.go for direct rule coverage.

	return nil
}

func (c *Config) validateQFDecisionsConfig() error {
	var configErrors []string
	if strings.TrimSpace(c.QFDecisionsBaseURL) == "" {
		configErrors = append(configErrors, "QF_DECISIONS_BASE_URL")
	} else if parsed, err := url.Parse(c.QFDecisionsBaseURL); err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		configErrors = append(configErrors, "QF_DECISIONS_BASE_URL (must be an absolute http or https URL)")
	}
	if strings.TrimSpace(c.QFDecisionsCredentialRef) == "" {
		configErrors = append(configErrors, "QF_DECISIONS_CREDENTIAL_REF")
	}
	if strings.TrimSpace(c.QFDecisionsSyncSchedule) == "" {
		configErrors = append(configErrors, "QF_DECISIONS_SYNC_SCHEDULE")
	} else if !isValidCronExpr(c.QFDecisionsSyncSchedule) {
		configErrors = append(configErrors, "QF_DECISIONS_SYNC_SCHEDULE (not a valid cron expression)")
	}
	if c.QFDecisionsPacketVersion < 1 {
		configErrors = append(configErrors, "QF_DECISIONS_PACKET_VERSION (must be a positive integer)")
	}
	if c.QFDecisionsPageSize < 1 || c.QFDecisionsPageSize > 100 {
		configErrors = append(configErrors, "QF_DECISIONS_PAGE_SIZE (must be an integer in range [1, 100])")
	}
	// BUG-020-010 — PERMISSIVE policy: empty signing-keys JSON is allowed
	// (preserves the "callback signing not configured in this environment"
	// deployment shape Scope 8 of spec 041 explicitly designed for). When
	// non-empty, the value MUST parse as a non-empty JSON array so a
	// malformed env var is surfaced at the consolidated Validate() choke
	// point rather than at Connect time.
	if raw := strings.TrimSpace(c.QFDecisionsCallbackSigningKeysJSON); raw != "" {
		var entries []json.RawMessage
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			configErrors = append(configErrors, fmt.Sprintf("QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON (must be a JSON array of {key_id,secret,not_before} entries: %v)", err))
		} else if len(entries) == 0 {
			configErrors = append(configErrors, "QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON (must be a non-empty JSON array)")
		}
	}
	if len(configErrors) > 0 {
		return fmt.Errorf("missing or invalid QF decisions connector configuration: %s", strings.Join(configErrors, ", "))
	}
	return nil
}

// cronFieldPattern matches a single cron field: number, *, ranges, steps, lists.
var cronFieldPattern = regexp.MustCompile(`^(\*|[0-9]+(-[0-9]+)?)((/[0-9]+)|(,[0-9]+(-[0-9]+)?)*)$`)

// isValidCronExpr validates a 5-field standard cron expression (minute hour dom month dow).
func isValidCronExpr(expr string) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false
	}
	for _, f := range fields {
		if !cronFieldPattern.MatchString(f) {
			return false
		}
	}
	return true
}

// parseEnvFloat reads an env var and returns its float64 value, or 0 if unset/invalid.
func parseEnvFloat(key string) float64 {
	s := os.Getenv(key)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseEnvJSONArray reads an env var containing a JSON array and returns []interface{}.
func parseEnvJSONArray(key string) []interface{} {
	s := os.Getenv(key)
	if s == "" {
		return nil
	}
	var result []interface{}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

// parseEnvJSONObject reads an env var containing a JSON object and returns map[string]interface{}.
func parseEnvJSONObject(key string) map[string]interface{} {
	s := os.Getenv(key)
	if s == "" {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

// mustParseIntEnv reads an env var as an int, returning a fail-loud error
// when the env var is unset/empty or unparseable. BUG-020-008 replaced the
// previous silent-default parseIntEnv helper with this fail-loud variant
// so each of the 8 SST-required int env vars surfaces at boot instead of
// silently substituting 0. The error message names the env key and (for
// parse failures) the offending value so the operator can fix every
// problem in one pass.
func mustParseIntEnv(key string) (int, error) {
	s := os.Getenv(key)
	if s == "" {
		return 0, fmt.Errorf("%s is required and unset", key)
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s has unparseable int value %q", key, s)
	}
	return v, nil
}

// parseComposeMemoryToMiB parses a docker-compose-style memory string
// (e.g. "512M", "1G", "3GB", "768MiB") into integer MiB. Returns an
// error when the input is unparseable so the caller can name the offending
// env var. Used by Load() for ML_MEMORY_LIMIT (spec 045 FR-045-002) so
// the Validate() chain can compare model profile MiB against the envelope.
//
// Recognized suffixes (case-insensitive):
//   - "k" / "kb" / "kib" — kibibytes
//   - "m" / "mb" / "mib" — mebibytes
//   - "g" / "gb" / "gib" — gibibytes
//   - "t" / "tb" / "tib" — tebibytes
//
// Compose treats both decimal (kB, MB, GB) and binary (KiB, MiB, GiB)
// suffixes as binary by convention (1G = 1024MiB), matching `docker info`
// and the deploy.resources.limits.memory contract. parseComposeMemoryToMiB
// follows the same convention.
func parseComposeMemoryToMiB(raw string) (int, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("empty memory value")
	}
	// Find the boundary between the numeric prefix and the unit suffix.
	cut := -1
	for i, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			continue
		}
		cut = i
		break
	}
	if cut == -1 {
		// All digits — bytes.
		bytesVal, err := strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("invalid byte count %q: %w", raw, err)
		}
		return bytesVal / (1024 * 1024), nil
	}
	numPart := s[:cut]
	unit := strings.ToLower(strings.TrimSpace(s[cut:]))
	num, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory numeric prefix %q: %w", numPart, err)
	}
	if num < 0 {
		return 0, fmt.Errorf("memory value %q must be non-negative", raw)
	}
	var multiplierMiB float64
	switch unit {
	case "k", "kb", "kib":
		multiplierMiB = 1.0 / 1024.0
	case "m", "mb", "mib":
		multiplierMiB = 1.0
	case "g", "gb", "gib":
		multiplierMiB = 1024.0
	case "t", "tb", "tib":
		multiplierMiB = 1024.0 * 1024.0
	default:
		return 0, fmt.Errorf("unrecognized memory unit %q in %q (expected k/m/g/t with optional b/ib suffix)", unit, raw)
	}
	mib := int(num * multiplierMiB)
	if mib <= 0 {
		return 0, fmt.Errorf("memory value %q resolves to non-positive MiB", raw)
	}
	return mib, nil
}

// validateModelEnvelopes enforces spec 045 FR-045-002 with the
// per-service envelope routing introduced by BUG-045-001:
//   - Every model env var configured for runtime use MUST have an
//     entry in the model_memory_profiles map.
//   - Every used model's required memory MUST fit within the
//     envelope of the deploy service that actually loads the model
//     at runtime — ollama-resident models against
//     c.OllamaMemoryLimitMiB (8 GiB on default home-lab config) and
//     ml-sidecar-resident models against c.MLMemoryLimitMiB (3 GiB
//     on default home-lab config).
//
// "Used models" are sourced from the SST runtime config. Buckets
// below cover every model field this Config struct surfaces that
// the Go core or ML sidecar will load at runtime. Empty values are
// skipped (some routes are optional and forward-compatible env
// vars resolve to empty until config.sh starts emitting them).
// Returns nil when every used model has a fitting profile, OR a
// fail-loud error naming every offender (with the correct envelope
// env var named per offender) so the operator can fix all problems
// in one pass.
func (c *Config) validateModelEnvelopes() error {
	if c.MLModelMemoryProfiles == nil {
		// Profile map missing is named by requiredVars() / Load()'s
		// JSON parse step. Defensive nil-guard.
		return nil
	}
	if c.MLMemoryLimitMiB == 0 && c.OllamaMemoryLimitMiB == 0 {
		// Both envelopes missing is named by requiredVars(); nothing
		// to do here.
		return nil
	}

	// Per-service routing: every model env var belongs to exactly one
	// deploy service. The bucket carries the envelope env-var name
	// (e.g. "OLLAMA_MEMORY_LIMIT"), its raw on-the-wire value (e.g.
	// "8G"), and the parsed MiB integer used for the fit check.
	type modelRef struct {
		envVar string
		model  string
	}
	type envelopeBucket struct {
		serviceName string // human-readable for the error message
		envelopeKey string // env var name of the envelope
		envelopeRaw string // raw compose-style envelope value
		envelopeMiB int    // parsed envelope MiB
		members     []modelRef
	}

	buckets := []envelopeBucket{
		{
			serviceName: "ollama",
			envelopeKey: "OLLAMA_MEMORY_LIMIT",
			envelopeRaw: c.OllamaMemoryLimit,
			envelopeMiB: c.OllamaMemoryLimitMiB,
			members: []modelRef{
				{"LLM_MODEL", c.LLMModel},
				{"OLLAMA_MODEL", c.OllamaModel},
				{"OLLAMA_VISION_MODEL", c.OllamaVisionModel},
				{"OLLAMA_OCR_MODEL", c.OllamaOcrModel},
				{"OLLAMA_REASONING_MODEL", c.OllamaReasoningModel},
				{"OLLAMA_FAST_MODEL", c.OllamaFastModel},
				{"PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", c.PhotosIntelligenceClassifyModel},
				{"PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL", c.PhotosIntelligenceSensitivityModel},
				{"PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", c.PhotosIntelligenceAestheticModel},
				{"PHOTOS_INTELLIGENCE_OCR_MODEL", c.PhotosIntelligenceOcrModel},
				{"AGENT_PROVIDER_DEFAULT_MODEL", c.AgentProviderDefaultModel},
				{"AGENT_PROVIDER_REASONING_MODEL", c.AgentProviderReasoningModel},
				{"AGENT_PROVIDER_FAST_MODEL", c.AgentProviderFastModel},
				{"AGENT_PROVIDER_VISION_MODEL", c.AgentProviderVisionModel},
				{"AGENT_PROVIDER_OCR_MODEL", c.AgentProviderOcrModel},
			},
		},
		{
			serviceName: "smackerel_ml",
			envelopeKey: "ML_MEMORY_LIMIT",
			envelopeRaw: c.MLMemoryLimit,
			envelopeMiB: c.MLMemoryLimitMiB,
			members: []modelRef{
				{"EMBEDDING_MODEL", c.EmbeddingModel},
				{"PHOTOS_INTELLIGENCE_EMBED_MODEL", c.PhotosIntelligenceEmbedModel},
			},
		},
	}

	var missing []string
	var oversized []string
	seen := make(map[string]struct{})
	for _, bucket := range buckets {
		for _, ref := range bucket.members {
			if ref.model == "" {
				continue
			}
			// De-duplicate by (envVar, model) so two routes onto the
			// same model still surface each route's envVar in the
			// error message but don't double-count the profile lookup.
			key := bucket.envelopeKey + "|" + ref.envVar + "|" + ref.model
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			profileMiB, ok := c.MLModelMemoryProfiles[ref.model]
			if !ok {
				missing = append(missing, fmt.Sprintf("%s=%q has no entry in services.ml.model_memory_profiles", ref.envVar, ref.model))
				continue
			}
			if bucket.envelopeMiB == 0 {
				// This bucket's envelope is missing; requiredVars()
				// names the envelope env var. Skip the fit check for
				// this bucket so the operator gets one clean missing-
				// envelope error instead of confusing per-model
				// "exceeds 0 MiB" noise.
				continue
			}
			if profileMiB > bucket.envelopeMiB {
				oversized = append(oversized, fmt.Sprintf("%s=%q requires %d MiB but %s=%q resolves to %d MiB", ref.envVar, ref.model, profileMiB, bucket.envelopeKey, bucket.envelopeRaw, bucket.envelopeMiB))
			}
		}
	}

	// Spec 082 SCOPE-082-02 — concurrent interactive-set envelope guard.
	// The per-model checks above ensure each model fits the ollama
	// envelope ALONE. But under a resident keep-alive (OLLAMA_KEEP_ALIVE
	// == "-1" or a long duration), ollama retains every model it loads,
	// so the distinct interactive hot-path models are co-resident and
	// their SUM must ALSO fit OLLAMA_MEMORY_LIMIT — otherwise Docker
	// OOM-kills the ollama container into a restart crash-loop. Only the
	// interactive hot-path slots (llm + ollama + ollama-vision + agent
	// default/fast/vision) are summed: these serve live conversational/
	// agent requests and are reliably co-resident. On-demand specialists
	// (reasoning, OCR, photo-intelligence batch) are governed by the
	// per-model individual check above plus operator keep-alive guidance,
	// and are not guaranteed to be co-resident at full ceiling.
	var concurrent []string
	if c.OllamaMemoryLimitMiB > 0 && ollamaKeepAliveResident(c.OllamaKeepAlive) {
		interactive := []modelRef{
			{"LLM_MODEL", c.LLMModel},
			{"OLLAMA_MODEL", c.OllamaModel},
			{"OLLAMA_VISION_MODEL", c.OllamaVisionModel},
			{"AGENT_PROVIDER_DEFAULT_MODEL", c.AgentProviderDefaultModel},
			{"AGENT_PROVIDER_FAST_MODEL", c.AgentProviderFastModel},
			{"AGENT_PROVIDER_VISION_MODEL", c.AgentProviderVisionModel},
		}
		residentSum := 0
		allProfiled := true
		seenModel := make(map[string]struct{})
		var residentNames []string
		for _, ref := range interactive {
			if ref.model == "" {
				continue
			}
			if _, dup := seenModel[ref.model]; dup {
				continue
			}
			seenModel[ref.model] = struct{}{}
			profileMiB, ok := c.MLModelMemoryProfiles[ref.model]
			if !ok {
				// Missing profile is already reported by the per-model
				// loop above; do not sum an unknown size.
				allProfiled = false
				continue
			}
			residentSum += profileMiB
			residentNames = append(residentNames, fmt.Sprintf("%s=%d MiB", ref.model, profileMiB))
		}
		if allProfiled && residentSum > c.OllamaMemoryLimitMiB {
			concurrent = append(concurrent, fmt.Sprintf(
				"interactive ollama working set {%s} sums to %d MiB but OLLAMA_MEMORY_LIMIT=%q resolves to %d MiB (keep-alive %q keeps these models co-resident; raise OLLAMA_MEMORY_LIMIT or shorten OLLAMA_KEEP_ALIVE)",
				strings.Join(residentNames, ", "), residentSum, c.OllamaMemoryLimit, c.OllamaMemoryLimitMiB, c.OllamaKeepAlive))
		}
	}

	// Spec 088 SCOPE-01 — switchable_models co-residence envelope guard.
	// The operator-curated assistant.open_knowledge.switchable_models set
	// is the allowlist of models /ask may be runtime-switched TO on the
	// forced-final SYNTHESIS turn. Each entry MUST have a memory profile
	// AND co-resident-fit the env ollama envelope alongside the gather
	// model (llm_model_id), which stays resident during synthesis — the
	// SAME arithmetic the runtime modelswitch.Allowlist uses. Checked only
	// when open-knowledge is enabled and the ollama envelope is known
	// (OllamaMemoryLimitMiB != 0 — e.g. home-lab; dev has no daemon and
	// resolves to 0, so the check is skipped there, matching the runtime
	// validator). FR-10 / SCN-088-A07: an operator cannot ship a
	// switchable list that busts the envelope.
	if c.Assistant.OpenKnowledge.Enabled && c.OllamaMemoryLimitMiB != 0 {
		gather := c.Assistant.OpenKnowledge.LLMModelID
		baseMiB := c.MLModelMemoryProfiles[gather]
		for _, m := range c.Assistant.OpenKnowledge.SwitchableModels {
			if strings.TrimSpace(m) == "" {
				continue // empty entry is reported by OpenKnowledgeConfig.Validate()
			}
			profileMiB, ok := c.MLModelMemoryProfiles[m]
			if !ok {
				missing = append(missing, fmt.Sprintf("assistant.open_knowledge.switchable_models entry %q has no entry in services.ml.model_memory_profiles", m))
				continue
			}
			coresident := baseMiB
			if m != gather {
				coresident += profileMiB
			}
			if coresident > c.OllamaMemoryLimitMiB {
				oversized = append(oversized, fmt.Sprintf("assistant.open_knowledge.switchable_models entry %q co-resident with gather model %q requires %d MiB but OLLAMA_MEMORY_LIMIT=%q resolves to %d MiB", m, gather, coresident, c.OllamaMemoryLimit, c.OllamaMemoryLimitMiB))
			}
		}
	}

	// Spec 089 SCOPE-01 — standing-default co-residence envelope guard
	// (closes the CT-6 gap). The spec-088 switchable pass above checks
	// only the runtime-switchable entries, NOT the STANDING DEFAULT
	// synthesis model (synthesis_model_id) that runs on EVERY /ask with
	// no override. Spec 088 got away with this because the home-lab
	// default (deepseek-r1:7b, 4864 MiB) is tiny; spec 089 promotes the
	// default to a large reasoning model (deepseek-r1:32b, 22528 MiB),
	// making the every-query model the ONE large selection that was NOT
	// envelope-checked. This guard applies the SAME co-residence
	// arithmetic the switchable pass uses — the resolved
	// synthesis_model_id co-resident with the gather model (llm_model_id)
	// MUST fit OllamaMemoryLimitMiB — so an over-envelope standing default
	// is refused fail-loud at config generation (FR-2 / SCN-089-A06).
	// Each tool_capable_gather_models entry MUST also be profiled (the
	// gather-override allowlist sanity, FR-8). Gated identically to the
	// switchable pass: only when open-knowledge is enabled and the ollama
	// envelope is known (dev has no daemon → OllamaMemoryLimitMiB == 0 →
	// skipped, matching the runtime validator).
	if c.Assistant.OpenKnowledge.Enabled && c.OllamaMemoryLimitMiB != 0 {
		gather := c.Assistant.OpenKnowledge.LLMModelID
		baseMiB := c.MLModelMemoryProfiles[gather]
		standingDefault := strings.TrimSpace(c.Assistant.OpenKnowledge.SynthesisModelID)
		if standingDefault != "" {
			profileMiB, ok := c.MLModelMemoryProfiles[standingDefault]
			if !ok {
				missing = append(missing, fmt.Sprintf("assistant.open_knowledge.synthesis_model_id (standing default) %q has no entry in services.ml.model_memory_profiles", standingDefault))
			} else {
				coresident := baseMiB
				if standingDefault != gather {
					coresident += profileMiB
				}
				if coresident > c.OllamaMemoryLimitMiB {
					oversized = append(oversized, fmt.Sprintf("assistant.open_knowledge.synthesis_model_id (standing default) %q co-resident with gather model %q requires %d MiB but OLLAMA_MEMORY_LIMIT=%q resolves to %d MiB", standingDefault, gather, coresident, c.OllamaMemoryLimit, c.OllamaMemoryLimitMiB))
				}
			}
		}
		// Each tool_capable_gather_models entry must have a memory profile
		// (a switchable gather model cannot be loaded if it is un-profiled).
		for _, m := range c.Assistant.OpenKnowledge.ToolCapableGatherModels {
			if strings.TrimSpace(m) == "" {
				continue // empty entry is reported by OpenKnowledgeConfig.Validate()
			}
			if _, ok := c.MLModelMemoryProfiles[m]; !ok {
				missing = append(missing, fmt.Sprintf("assistant.open_knowledge.tool_capable_gather_models entry %q has no entry in services.ml.model_memory_profiles", m))
			}
		}
	}

	if len(missing) > 0 || len(oversized) > 0 || len(concurrent) > 0 {
		var parts []string
		if len(missing) > 0 {
			parts = append(parts, "missing model memory profile(s): "+strings.Join(missing, "; "))
		}
		if len(oversized) > 0 {
			parts = append(parts, "model envelope exceeded: "+strings.Join(oversized, "; "))
		}
		if len(concurrent) > 0 {
			parts = append(parts, "concurrent ollama envelope exceeded: "+strings.Join(concurrent, "; "))
		}
		return fmt.Errorf("model envelope validation failed (spec 045 FR-045-002): %s", strings.Join(parts, " | "))
	}
	return nil
}

// ollamaKeepAliveResident reports whether an OLLAMA_KEEP_ALIVE value keeps
// loaded models resident long enough that the concurrent interactive set
// must be summed against the ollama envelope (Spec 082 SCOPE-082-02).
//
//   - "-1"                       → never unload → resident
//   - a Go duration ≥ 10m        → resident (e.g. "24h", "30m")
//   - "" / "0"                   → not resident (unset / evict immediately)
//   - a short duration (< 10m)   → not resident (e.g. "5m"; sporadic evict)
//
// An unparseable value is treated as NON-resident: the per-model check
// still applies, and we never fail-loud on a value we cannot interpret.
func ollamaKeepAliveResident(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return false
	}
	if raw == "-1" {
		return true
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return false
	}
	return d >= 10*time.Minute
}

// parseTelegramUserMapping parses a TELEGRAM_USER_MAPPING env value
// of the form "12345:alice,67890:bob" into a chat_id → user_id map.
//
// Spec 044 Scope 03 — claim-binding for the Telegram entry point.
// The format intentionally mirrors TELEGRAM_CHAT_IDS (comma-separated)
// so operators have a single mental model. Whitespace around tokens
// is tolerated; duplicate chat_ids fail loudly so a typo cannot
// silently re-attribute captures.
//
// This helper is duplicated in package telegram (telegram.ParseUserMapping)
// to keep the dep direction (telegram → config) clean. Both helpers
// MUST stay in sync; the unit tests in
// internal/telegram/user_mapping_test.go pin the canonical behavior.
func parseTelegramUserMapping(raw string) (map[int64]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	out := make(map[int64]string)
	for idx, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			return nil, fmt.Errorf("entry %d is empty (format: chat_id:user_id, comma-separated)", idx+1)
		}
		colon := strings.IndexByte(pair, ':')
		if colon <= 0 || colon == len(pair)-1 {
			return nil, fmt.Errorf("entry %d %q is malformed (expected chat_id:user_id)", idx+1, pair)
		}
		chatRaw := strings.TrimSpace(pair[:colon])
		userID := strings.TrimSpace(pair[colon+1:])
		if userID == "" {
			return nil, fmt.Errorf("entry %d has empty user_id", idx+1)
		}
		chatID, err := strconv.ParseInt(chatRaw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("entry %d chat_id %q is not int64: %w", idx+1, chatRaw, err)
		}
		if _, dup := out[chatID]; dup {
			return nil, fmt.Errorf("entry %d duplicates chat_id %d", idx+1, chatID)
		}
		out[chatID] = userID
	}
	return out, nil
}
