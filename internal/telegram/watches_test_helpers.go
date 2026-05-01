package telegram

// RenderWatchAlertForTests is a thin export that lets external test packages
// inspect the watch alert rendering without a live Telegram API connection.
// Spec 039 Scope 4 — used by the recommendations watches Telegram e2e test.
func RenderWatchAlertForTests(alert WatchAlert) string {
	return formatWatchAlertText(alert)
}
