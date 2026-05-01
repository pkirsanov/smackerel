package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

// WatchAlert is the renderer-safe payload the watch evaluator produces for
// Telegram delivery. It matches the design template:
//
//	<Title> | <Provider>
//	<Subtitle / labels>
//	Why? <Why>
//	[Open] [Why?] [Not interested] [Snooze 30d]
type WatchAlert struct {
	WatchID     string
	WatchName   string
	ActorUserID string
	Title       string
	Subtitle    string
	Provider    string
	Why         string
	Labels      []string
}

// WatchService is the persistence boundary used by the Telegram /watch
// command handler. It is implemented by *recstore.Store; the interface keeps
// the bot dispatch package independent of a concrete store type.
type WatchService interface {
	ListWatches(ctx context.Context, actorUserID string) ([]recstore.WatchRecord, error)
	FindWatchByName(ctx context.Context, actorUserID, name string) (recstore.WatchRecord, error)
	PauseWatch(ctx context.Context, id string, now time.Time) error
	ResumeWatch(ctx context.Context, id string, now time.Time) error
	SilenceWatch(ctx context.Context, id string, until time.Time, now time.Time) error
	DeleteWatch(ctx context.Context, id string, now time.Time) error
}

// SetWatchService installs the watch service used by the /watch command.
// MUST be called once during bot wiring; passing nil leaves /watch disabled.
func (b *Bot) SetWatchService(service WatchService) {
	b.watchService = service
}

// SetDefaultChatID configures the chat used to deliver scheduler-fired watch
// alerts when no per-actor mapping exists. Spec 039 Scope 4 ships single-tenant
// local actor; multi-actor routing arrives in a later scope.
func (b *Bot) SetDefaultChatID(chatID int64) {
	b.defaultChatID = chatID
}

// SendWatchAlert delivers a watch alert through the configured chat sender.
// Returns an error when the bot is not wired with a sender or actor mapping.
func (b *Bot) SendWatchAlert(ctx context.Context, alert WatchAlert) error {
	chatID := b.resolveActorChat(alert.ActorUserID)
	if chatID == 0 {
		return fmt.Errorf("watch alert: no chat mapping for actor %s", alert.ActorUserID)
	}
	text := formatWatchAlertText(alert)
	if b.api != nil {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = ""
		_, err := b.api.Send(msg)
		return err
	}
	return fmt.Errorf("watch alert: telegram bot not initialised")
}

func formatWatchAlertText(alert WatchAlert) string {
	lines := []string{}
	header := alert.Title
	if alert.Provider != "" {
		header = header + " | " + alert.Provider
	}
	lines = append(lines, MarkerInfo+header)
	if alert.Subtitle != "" {
		lines = append(lines, MarkerListItem+alert.Subtitle)
	}
	for _, label := range alert.Labels {
		lines = append(lines, MarkerListItem+label)
	}
	if alert.Why != "" {
		lines = append(lines, MarkerInfo+"Why? "+alert.Why)
	}
	lines = append(lines, MarkerListItem+"[Open] [Why?] [Not interested] [Snooze 30d]")
	return strings.Join(lines, "\n")
}

// handleWatchCommand handles the /watch command. Subcommands:
//
//	/watch list                — list saved watches
//	/watch pause <name>        — pause a watch by name
//	/watch resume <name>       — resume a watch by name
//	/watch silence <name> <h>  — silence a watch for <h> hours
//	/watch delete <name>       — DESTRUCTIVE — requires `confirm`
func (b *Bot) handleWatchCommand(ctx context.Context, msg *tgbotapi.Message, args string) {
	if b.watchService == nil {
		b.reply(msg.Chat.ID, MarkerUncertain+"Watches are not enabled.")
		return
	}
	parts := strings.Fields(args)
	if len(parts) == 0 {
		b.reply(msg.Chat.ID, MarkerInfo+"Watch commands: list | pause <name> | resume <name> | silence <name> <hours> | delete <name> confirm")
		return
	}
	subcommand := strings.ToLower(parts[0])
	rest := parts[1:]
	actor := b.resolveActor(msg)
	switch subcommand {
	case "list":
		b.handleWatchListCommand(ctx, msg, actor)
	case "pause":
		b.handleWatchPauseCommand(ctx, msg, actor, rest)
	case "resume":
		b.handleWatchResumeCommand(ctx, msg, actor, rest)
	case "silence":
		b.handleWatchSilenceCommand(ctx, msg, actor, rest)
	case "delete":
		b.handleWatchDeleteCommand(ctx, msg, actor, rest)
	default:
		b.reply(msg.Chat.ID, MarkerUncertain+"Unknown watch subcommand. Try list, pause, resume, silence, or delete.")
	}
}

func (b *Bot) handleWatchListCommand(ctx context.Context, msg *tgbotapi.Message, actor string) {
	records, err := b.watchService.ListWatches(ctx, actor)
	if err != nil {
		b.reply(msg.Chat.ID, MarkerUncertain+"Could not list watches: "+err.Error())
		return
	}
	if len(records) == 0 {
		b.reply(msg.Chat.ID, MarkerInfo+"No watches yet. Create one in the web UI under Recommendations > Watches.")
		return
	}
	lines := []string{MarkerHeading + "Your watches"}
	now := time.Now().UTC()
	for _, record := range records {
		state := "active"
		switch {
		case !record.Enabled:
			state = "paused"
		case record.SilenceUntil != nil && record.SilenceUntil.After(now):
			state = "silenced"
		}
		lines = append(lines, MarkerListItem+fmt.Sprintf("%s (%s) — %s — rate %d/%ds — last_run %s", record.Name, record.Kind, state, record.MaxAlertsPerWindow, record.AlertWindowSeconds, formatNullableTime(record.LastRunAt)))
	}
	b.reply(msg.Chat.ID, strings.Join(lines, "\n"))
}

func (b *Bot) handleWatchPauseCommand(ctx context.Context, msg *tgbotapi.Message, actor string, rest []string) {
	if len(rest) < 1 {
		b.reply(msg.Chat.ID, MarkerUncertain+"Usage: /watch pause <name>")
		return
	}
	name := strings.Join(rest, " ")
	record, err := b.watchService.FindWatchByName(ctx, actor, name)
	if err != nil {
		b.reply(msg.Chat.ID, MarkerUncertain+"Watch not found: "+name)
		return
	}
	if err := b.watchService.PauseWatch(ctx, record.ID, time.Now().UTC()); err != nil {
		b.reply(msg.Chat.ID, MarkerUncertain+"Pause failed: "+err.Error())
		return
	}
	b.reply(msg.Chat.ID, MarkerSuccess+"Paused "+record.Name)
}

func (b *Bot) handleWatchResumeCommand(ctx context.Context, msg *tgbotapi.Message, actor string, rest []string) {
	if len(rest) < 1 {
		b.reply(msg.Chat.ID, MarkerUncertain+"Usage: /watch resume <name>")
		return
	}
	name := strings.Join(rest, " ")
	record, err := b.watchService.FindWatchByName(ctx, actor, name)
	if err != nil {
		b.reply(msg.Chat.ID, MarkerUncertain+"Watch not found: "+name)
		return
	}
	if err := b.watchService.ResumeWatch(ctx, record.ID, time.Now().UTC()); err != nil {
		b.reply(msg.Chat.ID, MarkerUncertain+"Resume failed: "+err.Error())
		return
	}
	b.reply(msg.Chat.ID, MarkerSuccess+"Resumed "+record.Name)
}

func (b *Bot) handleWatchSilenceCommand(ctx context.Context, msg *tgbotapi.Message, actor string, rest []string) {
	if len(rest) < 2 {
		b.reply(msg.Chat.ID, MarkerUncertain+"Usage: /watch silence <name> <hours>")
		return
	}
	hoursIndex := len(rest) - 1
	hours, err := strconv.Atoi(rest[hoursIndex])
	if err != nil || hours < 1 || hours > 24*30 {
		b.reply(msg.Chat.ID, MarkerUncertain+"Hours must be an integer between 1 and 720.")
		return
	}
	name := strings.Join(rest[:hoursIndex], " ")
	record, err := b.watchService.FindWatchByName(ctx, actor, name)
	if err != nil {
		b.reply(msg.Chat.ID, MarkerUncertain+"Watch not found: "+name)
		return
	}
	now := time.Now().UTC()
	until := now.Add(time.Duration(hours) * time.Hour)
	if err := b.watchService.SilenceWatch(ctx, record.ID, until, now); err != nil {
		b.reply(msg.Chat.ID, MarkerUncertain+"Silence failed: "+err.Error())
		return
	}
	b.reply(msg.Chat.ID, MarkerSuccess+fmt.Sprintf("Silenced %s for %d hours (until %s)", record.Name, hours, until.Format(time.RFC3339)))
}

func (b *Bot) handleWatchDeleteCommand(ctx context.Context, msg *tgbotapi.Message, actor string, rest []string) {
	if len(rest) < 1 {
		b.reply(msg.Chat.ID, MarkerUncertain+"Usage: /watch delete <name> confirm")
		return
	}
	confirmIndex := -1
	for i, token := range rest {
		if strings.EqualFold(token, "confirm") {
			confirmIndex = i
			break
		}
	}
	if confirmIndex < 0 {
		name := strings.Join(rest, " ")
		b.reply(msg.Chat.ID, MarkerUncertain+fmt.Sprintf("Delete is destructive. Re-send: /watch delete %s confirm", name))
		return
	}
	if confirmIndex == 0 {
		b.reply(msg.Chat.ID, MarkerUncertain+"Usage: /watch delete <name> confirm")
		return
	}
	name := strings.Join(rest[:confirmIndex], " ")
	record, err := b.watchService.FindWatchByName(ctx, actor, name)
	if err != nil {
		b.reply(msg.Chat.ID, MarkerUncertain+"Watch not found: "+name)
		return
	}
	if err := b.watchService.DeleteWatch(ctx, record.ID, time.Now().UTC()); err != nil {
		b.reply(msg.Chat.ID, MarkerUncertain+"Delete failed: "+err.Error())
		return
	}
	b.reply(msg.Chat.ID, MarkerSuccess+"Deleted "+record.Name)
}

func (b *Bot) resolveActor(_ *tgbotapi.Message) string {
	// Spec 039 Scope 4 ships single-tenant local actor. Multi-tenant actor
	// resolution is a future scope responsibility.
	return "local"
}

func (b *Bot) resolveActorChat(_ string) int64 {
	if b.defaultChatID != 0 {
		return b.defaultChatID
	}
	return 0
}

func formatNullableTime(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return t.Format(time.RFC3339)
}
