package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/connector"
	alertsConnector "github.com/smackerel/smackerel/internal/connector/alerts"
	bookmarksConnector "github.com/smackerel/smackerel/internal/connector/bookmarks"
	browserConnector "github.com/smackerel/smackerel/internal/connector/browser"
	caldavConnector "github.com/smackerel/smackerel/internal/connector/caldav"
	discordConnector "github.com/smackerel/smackerel/internal/connector/discord"
	guesthostConnector "github.com/smackerel/smackerel/internal/connector/guesthost"
	hospitableConnector "github.com/smackerel/smackerel/internal/connector/hospitable"
	imapConnector "github.com/smackerel/smackerel/internal/connector/imap"
	keepConnector "github.com/smackerel/smackerel/internal/connector/keep"
	mapsConnector "github.com/smackerel/smackerel/internal/connector/maps"
	marketsConnector "github.com/smackerel/smackerel/internal/connector/markets"
	rssConnector "github.com/smackerel/smackerel/internal/connector/rss"
	twitterConnector "github.com/smackerel/smackerel/internal/connector/twitter"
	weatherConnector "github.com/smackerel/smackerel/internal/connector/weather"
	youtubeConnector "github.com/smackerel/smackerel/internal/connector/youtube"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// registerConnectors creates, registers, and auto-starts all connectors.
func registerConnectors(ctx context.Context, cfg *config.Config, svc *coreServices) error {
	// Instantiate all connectors
	imapConn := imapConnector.New("gmail")
	caldavConn := caldavConnector.New("google-calendar")
	ytConn := youtubeConnector.New("youtube")
	rssConn := rssConnector.New("rss", nil) // feed URLs configured via source_config
	keepConn := keepConnector.New("google-keep")
	bmConn := bookmarksConnector.NewConnectorWithPool("bookmarks", svc.pg.Pool)
	browserHistConn := browserConnector.New("browser-history")
	mapsConn := mapsConnector.New("google-maps-timeline")
	mapsConn.SetPool(svc.pg.Pool) // Enable pattern detection (commute, trip, temporal-spatial linking)
	hospitableConn := hospitableConnector.New("hospitable")
	guesthostConn := guesthostConnector.New()
	discordConn := discordConnector.New("discord")
	twitterConn := twitterConnector.New("twitter")
	weatherConn := weatherConnector.New("weather")
	alertsConn := alertsConnector.New("gov-alerts")
	marketsConn := marketsConnector.New("financial-markets")
	for _, c := range []connector.Connector{
		imapConn, caldavConn, ytConn, rssConn, keepConn,
		bmConn, browserHistConn, mapsConn, hospitableConn, guesthostConn,
		discordConn, twitterConn, weatherConn, alertsConn, marketsConn,
	} {
		if err := svc.registry.Register(c); err != nil {
			return fmt.Errorf("register connector %q: %w", c.ID(), err)
		}
	}
	slog.Info("connector registry initialized", "count", svc.registry.Count())

	// Auto-start bookmarks connector (no OAuth needed — file-based import)
	if cfg.BookmarksEnabled && cfg.BookmarksImportDir != "" {
		processingTier := cfg.BookmarksProcessingTier
		if processingTier == "" {
			processingTier = "full"
		}
		bmConfig := connector.ConnectorConfig{
			AuthType:       "none",
			Enabled:        true,
			ProcessingTier: processingTier,
			SyncSchedule:   cfg.BookmarksSyncSchedule,
			SourceConfig: map[string]interface{}{
				"import_dir":        cfg.BookmarksImportDir,
				"watch_interval":    cfg.BookmarksWatchInterval,
				"archive_processed": cfg.BookmarksArchiveProcessed,
				"min_url_length":    cfg.BookmarksMinURLLength,
				"exclude_domains":   parseJSONArray(cfg.BookmarksExcludeDomains),
			},
		}
		if err := bmConn.Connect(ctx, bmConfig); err == nil {
			svc.supervisor.SetConfig("bookmarks", bmConfig)
			svc.supervisor.StartConnector(ctx, "bookmarks")
			slog.Info("bookmarks connector started", "import_dir", cfg.BookmarksImportDir)
		} else {
			slog.Warn("bookmarks connector failed to start", "error", err)
		}
	}

	// Auto-start browser history connector (no OAuth needed — file-based)
	if cfg.BrowserHistoryPath != "" {
		browserCfg := connector.ConnectorConfig{
			AuthType: "none",
			Enabled:  true,
			SourceConfig: map[string]interface{}{
				"history_path": cfg.BrowserHistoryPath,
			},
		}
		if err := browserHistConn.Connect(ctx, browserCfg); err == nil {
			svc.supervisor.SetConfig("browser-history", browserCfg)
			svc.supervisor.StartConnector(ctx, "browser-history")
			slog.Info("browser history connector started", "history_path", cfg.BrowserHistoryPath)
		} else {
			slog.Warn("browser history connector failed to start", "error", err)
		}
	}

	// Auto-start Google Maps Timeline connector (no OAuth needed — file-based Takeout import)
	if cfg.MapsImportDir != "" {
		mapsCfg := connector.ConnectorConfig{
			AuthType:     "none",
			Enabled:      true,
			SyncSchedule: cfg.MapsSyncSchedule,
			SourceConfig: map[string]interface{}{
				"import_dir":               cfg.MapsImportDir,
				"watch_interval":           cfg.MapsWatchInterval,
				"archive_processed":        cfg.MapsArchiveProcessed,
				"min_distance_m":           cfg.MapsMinDistanceM,
				"min_duration_min":         cfg.MapsMinDurationMin,
				"location_radius_m":        cfg.MapsLocationRadiusM,
				"home_detection":           cfg.MapsHomeDetection,
				"commute_min_occurrences":  cfg.MapsCommuteMinOccurrences,
				"commute_window_days":      cfg.MapsCommuteWindowDays,
				"commute_weekdays_only":    cfg.MapsCommuteWeekdaysOnly,
				"trip_min_distance_km":     cfg.MapsTripMinDistanceKm,
				"trip_min_overnight_hours": cfg.MapsTripMinOvernightHours,
				"link_time_extend_min":     cfg.MapsLinkTimeExtendMin,
				"link_proximity_radius_m":  cfg.MapsLinkProximityRadiusM,
			},
		}
		if err := mapsConn.Connect(ctx, mapsCfg); err == nil {
			svc.supervisor.SetConfig("google-maps-timeline", mapsCfg)
			svc.supervisor.StartConnector(ctx, "google-maps-timeline")
			slog.Info("google maps timeline connector started", "import_dir", cfg.MapsImportDir)
		} else {
			slog.Warn("google maps timeline connector failed to start", "error", err)
		}
	}

	// Auto-start Discord connector (token-based)
	if cfg.DiscordEnabled {
		discordCfg := connector.ConnectorConfig{
			AuthType:     "token",
			Credentials:  map[string]string{"bot_token": cfg.DiscordBotToken},
			Enabled:      true,
			SyncSchedule: cfg.DiscordSyncSchedule,
			SourceConfig: map[string]interface{}{
				"enable_gateway":     cfg.DiscordEnableGateway,
				"backfill_limit":     cfg.DiscordBackfillLimit,
				"include_threads":    cfg.DiscordIncludeThreads,
				"include_pins":       cfg.DiscordIncludePins,
				"capture_commands":   cfg.DiscordCaptureCommands,
				"monitored_channels": cfg.DiscordMonitoredChannels,
			},
		}
		if err := discordConn.Connect(ctx, discordCfg); err == nil {
			svc.supervisor.SetConfig("discord", discordCfg)
			svc.supervisor.StartConnector(ctx, "discord")
			slog.Info("discord connector started")
		} else {
			slog.Warn("discord connector failed to start", "error", err)
		}
	}

	// Auto-start Twitter/X connector (token or file-based)
	if cfg.TwitterEnabled {
		twitterCfg := connector.ConnectorConfig{
			AuthType:     "token",
			Credentials:  map[string]string{"bearer_token": cfg.TwitterBearerToken},
			Enabled:      true,
			SyncSchedule: cfg.TwitterSyncSchedule,
			SourceConfig: map[string]interface{}{
				"sync_mode":   cfg.TwitterSyncMode,
				"archive_dir": cfg.TwitterArchiveDir,
			},
		}
		if err := twitterConn.Connect(ctx, twitterCfg); err == nil {
			svc.supervisor.SetConfig("twitter", twitterCfg)
			svc.supervisor.StartConnector(ctx, "twitter")
			slog.Info("twitter connector started")
		} else {
			slog.Warn("twitter connector failed to start", "error", err)
		}
	}

	// Auto-start Weather connector (no auth — Open-Meteo is free)
	if cfg.WeatherEnabled {
		weatherCfg := connector.ConnectorConfig{
			AuthType:     "none",
			Enabled:      true,
			SyncSchedule: cfg.WeatherSyncSchedule,
			SourceConfig: map[string]interface{}{
				"locations":     cfg.WeatherLocations,
				"enable_alerts": cfg.WeatherEnableAlerts,
				"forecast_days": cfg.WeatherForecastDays,
				"precision":     cfg.WeatherPrecision,
			},
		}
		if err := weatherConn.Connect(ctx, weatherCfg); err == nil {
			svc.supervisor.SetConfig("weather", weatherCfg)
			svc.supervisor.StartConnector(ctx, "weather")
			slog.Info("weather connector started")
		} else {
			slog.Warn("weather connector failed to start", "error", err)
		}
	}

	// Auto-start Gov Alerts connector (no auth — USGS/NWS are free)
	if cfg.GovAlertsEnabled {
		// Wire proactive alert notifier to publish extreme/severe alerts to NATS
		alertsConn.Notifier = &alertsConnector.NATSAlertNotifier{
			PublishFn: svc.nc.Publish,
			Subject:   smacknats.SubjectAlertsNotify,
		}

		alertsCfg := connector.ConnectorConfig{
			AuthType:     "api_key",
			Credentials:  map[string]string{"airnow_api_key": cfg.GovAlertsAirnowAPIKey},
			Enabled:      true,
			SyncSchedule: cfg.GovAlertsSyncSchedule,
			SourceConfig: map[string]interface{}{
				"locations":                cfg.GovAlertsLocations,
				"min_earthquake_magnitude": cfg.GovAlertsMinEarthquakeMag,
				"travel_locations":         cfg.GovAlertsTravelLocations,
				"source_earthquake":        cfg.GovAlertsSourceEarthquake,
				"source_weather":           cfg.GovAlertsSourceWeather,
				"source_tsunami":           cfg.GovAlertsSourceTsunami,
				"source_volcano":           cfg.GovAlertsSourceVolcano,
				"source_wildfire":          cfg.GovAlertsSourceWildfire,
				"source_airnow":            cfg.GovAlertsSourceAirnow,
				"source_gdacs":             cfg.GovAlertsSourceGdacs,
			},
		}
		if err := alertsConn.Connect(ctx, alertsCfg); err == nil {
			svc.supervisor.SetConfig("gov-alerts", alertsCfg)
			svc.supervisor.StartConnector(ctx, "gov-alerts")
			slog.Info("gov-alerts connector started")
		} else {
			slog.Warn("gov-alerts connector failed to start", "error", err)
		}
	}

	// Auto-start Financial Markets connector (API key auth)
	if cfg.FinancialMarketsEnabled {
		marketsCfg := connector.ConnectorConfig{
			AuthType: "api_key",
			Credentials: map[string]string{
				"finnhub_api_key": cfg.FinancialMarketsFinnhubAPIKey,
				"fred_api_key":    cfg.FinancialMarketsFredAPIKey,
			},
			Enabled:      true,
			SyncSchedule: cfg.FinancialMarketsSyncSchedule,
			SourceConfig: map[string]interface{}{
				"watchlist":         cfg.FinancialMarketsWatchlist,
				"alert_threshold":   cfg.FinancialMarketsAlertThresh,
				"coingecko_enabled": cfg.FinancialMarketsCoingecko,
				"fred_enabled":      cfg.FinancialMarketsFredEnabled,
				"fred_series":       cfg.FinancialMarketsFredSeries,
			},
		}
		if err := marketsConn.Connect(ctx, marketsCfg); err == nil {
			svc.supervisor.SetConfig("financial-markets", marketsCfg)
			svc.supervisor.StartConnector(ctx, "financial-markets")
			slog.Info("financial-markets connector started")
		} else {
			slog.Warn("financial-markets connector failed to start", "error", err)
		}
	}

	// Start connectors that have valid OAuth tokens
	if svc.tokenStore.HasToken(ctx, "google") {
		token, err := svc.tokenStore.Get(ctx, "google")
		if err == nil && token != nil {
			creds := map[string]string{"access_token": token.AccessToken}
			imapConfig := connector.ConnectorConfig{
				AuthType:     "oauth2",
				Credentials:  creds,
				Enabled:      true,
				SyncSchedule: cfg.IMAPSyncSchedule,
			}
			if err := imapConn.Connect(ctx, imapConfig); err == nil {
				svc.supervisor.SetConfig("gmail", imapConfig)
				svc.supervisor.StartConnector(ctx, "gmail")
				slog.Info("gmail connector started with OAuth token")
			}
			caldavConfig := connector.ConnectorConfig{
				AuthType:     "oauth2",
				Credentials:  creds,
				Enabled:      true,
				SyncSchedule: cfg.CalDAVSyncSchedule,
			}
			if err := caldavConn.Connect(ctx, caldavConfig); err == nil {
				svc.supervisor.SetConfig("google-calendar", caldavConfig)
				svc.supervisor.StartConnector(ctx, "google-calendar")
				slog.Info("google-calendar connector started with OAuth token")
			}
			ytConfig := connector.ConnectorConfig{
				AuthType:     "oauth2",
				Credentials:  creds,
				Enabled:      true,
				SyncSchedule: cfg.YouTubeSyncSchedule,
			}
			if err := ytConn.Connect(ctx, ytConfig); err == nil {
				svc.supervisor.SetConfig("youtube", ytConfig)
				svc.supervisor.StartConnector(ctx, "youtube")
				slog.Info("youtube connector started with OAuth token")
			}
		}
	} else {
		slog.Info("no Google OAuth token found — connectors will start when user authorizes via /auth/google/start")
	}

	return nil
}
