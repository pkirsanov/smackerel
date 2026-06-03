// Spec 066 SCOPE-2 — wires the Telegram retired-alias interceptor.
//
// The interceptor composes spec 075's LegacyRetirementConfig
// (catalog), spec 075's WindowStateResolver (already wired by
// startTelegramBotIfConfigured for the SCOPE-1 BotCommands menu),
// and spec 075's NoticeLedger (SQL-backed when a postgres pool is
// available; in-memory otherwise so dev/test installs still exercise
// the rewrite + closed-window paths). No spec-066-owned SST keys,
// tables, or migrations are introduced.
//
// This function is intentionally tolerant of partial wiring: every
// construction failure logs at WARN and leaves the interceptor
// unwired so the bot still serves traffic via the legacy command
// handlers (BS-001 regression-safe fallthrough). Hard failure would
// only be appropriate after the spec 066 closed-window cutover.
package main

import (
	"log/slog"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/telegram"
)

func wireLegacyAliasInterceptor(cfg *config.Config, svc *coreServices, tgBot *telegram.Bot) {
	if tgBot == nil {
		return
	}
	// Spec 076 SCOPE-6a — when a postgres pool is available, use the
	// durable SQLPauseStateStore so the interceptor observes the
	// same threshold-evaluator-driven pause transitions as the
	// facade. Without a pool (dev/test install with no DB) we fall
	// back to the static not-paused reader so the interceptor still
	// exercises the rewrite + closed-window paths.
	var pauseReader legacyretirement.PauseStateReader
	if svc != nil && svc.pg != nil && svc.pg.Pool != nil {
		store, perr := legacyretirement.NewSQLPauseStateStore(svc.pg.Pool)
		if perr != nil {
			slog.Warn("legacy alias interceptor: SQL pause-state store construction failed; falling back to static reader",
				"error", perr)
			pauseReader = legacyretirement.NewStaticPauseStateReader(false)
		} else {
			pauseReader = store
		}
	} else {
		pauseReader = legacyretirement.NewStaticPauseStateReader(false)
	}
	resolver, err := legacyretirement.NewWindowStateResolver(
		legacyretirement.SSTStateConfig{
			WindowID:    cfg.LegacyRetirement.WindowID,
			WindowState: cfg.LegacyRetirement.WindowState,
		},
		pauseReader,
	)
	if err != nil {
		slog.Warn("legacy alias interceptor: window-state resolver construction failed; interceptor not wired",
			"error", err)
		return
	}
	catalog, err := legacyretirement.NewConfigCatalog(legacyretirement.CatalogConfig{
		NoticeCopyPerCommand:          cfg.LegacyRetirement.NoticeCopyPerCommand,
		PostWindowUnknownResponseCopy: cfg.LegacyRetirement.PostWindowUnknownResponseCopy,
	})
	if err != nil {
		slog.Warn("legacy alias interceptor: catalog construction failed; interceptor not wired",
			"error", err)
		return
	}
	var ledger legacyretirement.NoticeLedger
	if svc != nil && svc.pg != nil && svc.pg.Pool != nil {
		sqlLedger, lerr := legacyretirement.NewSQLNoticeLedger(svc.pg.Pool)
		if lerr != nil {
			slog.Warn("legacy alias interceptor: SQL ledger construction failed; falling back to in-memory ledger",
				"error", lerr)
			ledger = legacyretirement.NewInMemoryNoticeLedger()
		} else {
			ledger = sqlLedger
		}
	} else {
		ledger = legacyretirement.NewInMemoryNoticeLedger()
	}
	hasher, err := legacyretirement.NewUserBucketHasher(cfg.LegacyRetirement.UserBucketHMACKey)
	if err != nil {
		slog.Warn("legacy alias interceptor: user-bucket hasher construction failed; interceptor not wired",
			"error", err)
		return
	}
	policy, err := legacyretirement.NewPolicy(legacyretirement.PolicyConfig{
		Catalog:       catalog,
		Ledger:        ledger,
		StateResolver: resolver,
		BucketHasher:  hasher,
		WindowID:      cfg.LegacyRetirement.WindowID,
	})
	if err != nil {
		slog.Warn("legacy alias interceptor: policy construction failed; interceptor not wired",
			"error", err)
		return
	}
	interceptor, err := telegram.NewLegacyAliasInterceptor(policy, nil)
	if err != nil {
		slog.Warn("legacy alias interceptor: construction failed; interceptor not wired",
			"error", err)
		return
	}
	tgBot.SetLegacyAliasInterceptor(interceptor)
	slog.Info("telegram legacy alias interceptor wired (spec 066 SCOPE-2)")
}
