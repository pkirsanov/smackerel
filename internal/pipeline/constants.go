package pipeline

// ProcessingStatus represents the processing state of an artifact.
type ProcessingStatus string

const (
	StatusPending   ProcessingStatus = "pending"
	StatusProcessed ProcessingStatus = "processed"
	StatusFailed    ProcessingStatus = "failed"
)

// Source ID constants — canonical source identifiers shared across capture API,
// Telegram bot, connectors, and tier assignment. Defined here (not in processor.go)
// so adding a new connector does not require editing the processor.
const (
	SourceCapture        = "capture"
	SourceTelegram       = "telegram"
	SourceBrowser        = "browser"
	SourceBrowserHistory = "browser-history"
	SourceRSS            = "rss"
	SourceBookmarks      = "bookmarks"
	SourceGoogleKeep     = "google-keep"
	SourceGoogleMaps     = "google-maps-timeline"
	SourceHospitable     = "hospitable"
	SourceGmail            = "gmail"
	SourceGoogleCalendar   = "google-calendar"
	SourceYouTube          = "youtube"
	SourceDiscord          = "discord"
	SourceTwitter          = "twitter"
	SourceWeather          = "weather"
	SourceGovAlerts        = "gov-alerts"
	SourceFinancialMarkets = "financial-markets"
	SourceGuestHost        = "guesthost"
)
