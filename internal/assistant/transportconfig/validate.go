package transportconfig

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// LookupEnvFn is the env-lookup signature consumed by Validate. It
// mirrors os.LookupEnv so the production call site is a one-liner
// and tests can inject a fake env map. The bool return distinguishes
// "key missing entirely" (SST contract violation) from "key present
// but empty" (sometimes legal, e.g. cors_allowed_origins).
type LookupEnvFn func(envVar string) (string, bool)

// Validate walks Registry filtered by transport and returns the first
// required entry whose env var is MISSING (LookupEnv ok==false).
// Entries with non-empty DefaultedFor are ratified exceptions and are
// not enforced here — their owning adapter performs the conditional
// runtime check. The returned error carries the entry's FailLoudMsg
// verbatim, matching the operator-facing string documented in the
// registry (spec 062 SCOPE-2).
func Validate(transport string, lookup LookupEnvFn) error {
	if lookup == nil {
		return errors.New("transportconfig.Validate: lookup is required")
	}
	entries := entriesForTransport(transport)
	if len(entries) == 0 {
		return fmt.Errorf("transportconfig.Validate: unknown transport %q", transport)
	}
	for _, e := range entries {
		if !e.Required || e.DefaultedFor != "" {
			continue
		}
		if _, ok := lookup(e.EnvVar); !ok {
			return errors.New(e.FailLoudMsg)
		}
	}
	return nil
}

// ValidateOwningPackage walks Registry filtered by OwningPackage
// (matches the OwningPackage field exactly) and returns the first
// missing REQUIRED env var as a FailLoudMsg error. Used by adapter
// packages that share a Transport tag with another owner but need
// to validate only the keys they consume (e.g. internal/telegram
// vs internal/telegram/assistant_adapter both tagged "telegram").
func ValidateOwningPackage(owningPackage string, lookup LookupEnvFn) error {
	if lookup == nil {
		return errors.New("transportconfig.ValidateOwningPackage: lookup is required")
	}
	matched := 0
	for _, e := range Registry {
		if e.OwningPackage != owningPackage {
			continue
		}
		matched++
		if !e.Required || e.DefaultedFor != "" {
			continue
		}
		if _, ok := lookup(e.EnvVar); !ok {
			return errors.New(e.FailLoudMsg)
		}
	}
	if matched == 0 {
		return fmt.Errorf("transportconfig.ValidateOwningPackage: no entries for owning package %q", owningPackage)
	}
	return nil
}

// ValidateAll runs Validate for every transport namespace in
// registry order ("http", "whatsapp", "telegram"). It returns the
// first failure so the operator sees one fail-loud message per boot
// attempt; subsequent failures are exposed on the next restart.
func ValidateAll(lookup LookupEnvFn) error {
	for _, t := range orderedTransports() {
		if err := Validate(t, lookup); err != nil {
			return err
		}
	}
	return nil
}

// ValidateAllFromOSEnv is the production-call shortcut used by
// cmd/core/main.go at startup. Kept as a named helper so a grep for
// "transportconfig.ValidateAllFromOSEnv" pinpoints every adapter
// boot-time consumer.
func ValidateAllFromOSEnv() error {
	return ValidateAll(os.LookupEnv)
}

func entriesForTransport(transport string) []Entry {
	var out []Entry
	for _, e := range Registry {
		if e.Transport == transport {
			out = append(out, e)
		}
	}
	return out
}

func orderedTransports() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, e := range Registry {
		if _, ok := seen[e.Transport]; ok {
			continue
		}
		seen[e.Transport] = struct{}{}
		out = append(out, e.Transport)
	}
	sort.Strings(out)
	return out
}

// RequiredEnvVars returns the env-var names for every Required,
// non-DefaultedFor entry filtered by transport. Used by SCN-062-A05
// (e2e) to enumerate which keys must each individually trigger a
// fail-loud exit when removed. Order is registry-order.
func RequiredEnvVars(transport string) []string {
	var out []string
	for _, e := range entriesForTransport(transport) {
		if !e.Required || e.DefaultedFor != "" {
			continue
		}
		out = append(out, e.EnvVar)
	}
	return out
}

// FailLoudMessageFor returns the registry FailLoudMsg for the given
// env-var name. Returns "" when no matching entry exists. Used by
// owning packages that want to delegate but also surface the literal
// message in a context-specific log line.
func FailLoudMessageFor(envVar string) string {
	for _, e := range Registry {
		if e.EnvVar == envVar {
			return e.FailLoudMsg
		}
	}
	return ""
}

// init enforces an internal invariant: every Registry entry's
// FailLoudMsg starts with the YAMLKey, so the operator-visible
// message always names the offender. A panic here is a developer
// error caught at process start, never at customer runtime (the
// Registry is package-level and built from constant data).
func init() {
	var bad []string
	for _, e := range Registry {
		if !strings.HasPrefix(e.FailLoudMsg, e.YAMLKey+" ") {
			bad = append(bad, e.YAMLKey)
		}
	}
	if len(bad) > 0 {
		panic("transportconfig.Registry: FailLoudMsg must start with YAMLKey for: " + strings.Join(bad, ", "))
	}
}
