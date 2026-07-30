// Package configgen provides the hermetic environment that every Go test
// harness must hand to scripts/commands/config.sh.
//
// The SST loader is fail-loud by design (smackerel-no-defaults): it aborts
// when a required operator input is absent rather than substituting a hidden
// default. Two such inputs have no committed value and therefore no value a
// test can inherit from a clean container env:
//
//   - SMACKEREL_HARDWARE_TIER — [F061-HARDWARE-TIER-MISSING]
//   - SMACKEREL_OLLAMA_URL    — [F-OLLAMA-URL-MISSING]
//
// The Ollama daemon address is per-operator deployment topology and is
// deliberately NOT committed to this repo (product-deployment-boundary), so
// config/smackerel.yaml carries llm.ollama_url: "" and the operator supplies
// the real address out of band (the gitignored .smackerel.local.env for dev,
// the deploy adapter's app.env injection for self-hosted).
//
// Tests must depend on neither: they must not require a reachable daemon and
// must not embed the operator's real address. HermeticEnv supplies a synthetic
// stand-in so the loader clears its guards and proceeds to the behavior under
// test. It is a TEST FIXTURE, not a default — the guards in config.sh are
// untouched and still abort when the value is genuinely absent, which
// TestConfigGenerate_AbortsWhenOllamaURLAbsent proves.
package configgen

// SyntheticOllamaURL is an unreachable placeholder daemon address. The
// .invalid TLD is reserved by RFC 2606 and is guaranteed never to resolve, so
// a test that accidentally starts depending on a live daemon fails loudly
// instead of silently talking to whatever is listening.
const SyntheticOllamaURL = "http://ollama.invalid:11434"

// HermeticEnv returns the KEY=VALUE pins required to run the SST loader from a
// test. Append it AFTER os.Environ() so the pins win over any ambient value a
// developer happens to have exported locally, keeping the harness hermetic.
func HermeticEnv() []string {
	return []string{
		"SMACKEREL_HARDWARE_TIER=cpu",
		"SMACKEREL_OLLAMA_URL=" + SyntheticOllamaURL,
	}
}
