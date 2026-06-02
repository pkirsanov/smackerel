// Package transportconfig is the per-transport configuration registry for
// the assistant's three transports (HTTP, WhatsApp, Telegram). Spec 062
// SCOPE-1 introduces this package as inventory-only: it enumerates the
// YAML keys + env-var bindings that already exist in config/smackerel.yaml
// and scripts/commands/config.sh. SCOPE-2 will wire adapter startup to
// drive fail-loud checks off this registry; SCOPE-3 will publish the
// operator-facing docs/Transport_Configuration.md mirror.
package transportconfig

// Entry is a single per-transport configuration key. See spec 062
// design.md §3 for the canonical shape.
type Entry struct {
	Transport     string // "http" | "whatsapp" | "telegram"
	YAMLKey       string // dotted YAML path, e.g. "assistant.transports.http.shared_user_id"
	EnvVar        string // generator-emitted env var name
	Required      bool   // true ⇒ generator uses required_value (fail-loud at config generate)
	FailLoudMsg   string // exact message the adapter MUST print when the value is missing/empty
	OwningPackage string // adapter package consuming this key
	IntroducedBy  string // spec + scope that introduced the key
	DefaultedFor  string // non-empty ⇒ explicitly ratified default (no silent carry-over)
}

// TransportNamespaces enumerates the YAML namespace prefixes that the
// registry claims to cover. The registry test (SCN-062-A01) walks
// config/smackerel.yaml under each prefix and asserts every leaf key
// has a registry entry.
var TransportNamespaces = []string{
	"assistant.transports.http",
	"assistant.transports.whatsapp",
	"assistant.transports.telegram",
	"telegram",
}

// Registry is the union of all per-transport entries. Order is stable
// and grouped by transport for readability; tests treat it as a set.
var Registry = func() []Entry {
	var out []Entry
	out = append(out, httpEntries...)
	out = append(out, whatsappEntries...)
	out = append(out, telegramEntries...)
	return out
}()
