// config-validate is the pre-emit check invoked from
// scripts/commands/config.sh BEFORE the generated env file is
// atomically promoted from <env>.env.tmp to <env>.env.
//
// It reads a single env file (one KEY=VALUE per line; comments and
// blank lines skipped; outer "..." or '...' quotes stripped from
// values), populates os.Environ via os.Setenv, and then runs the
// canonical Smackerel runtime validators (`config.Load()` then
// `Config.Validate()`). Exit codes:
//
//	0 — env file is valid
//	1 — Load() or Validate() rejected the env file (error to stderr)
//	2 — usage error or env-file unreadable (error to stderr)
//
// Spec 045 BUG-045-001 Scope 2 / DD-2: this binary is the
// single-source-of-truth gate that reuses the runtime Validate()
// path at config-generate time, so the operator sees the
// per-service envelope rejection at `./smackerel.sh config generate`
// time instead of at smackerel-core startup. The shell wrapper is
// fail-loud (it removes the TEMP env file on rejection and exits
// non-zero, propagating this binary's stderr to the operator).
//
// References:
//   - specs/045-deploy-resource-filesystem-hardening/bugs/
//     BUG-045-001-ml-envelope-cross-service-routing/design.md DD-2
//   - specs/045-deploy-resource-filesystem-hardening/bugs/
//     BUG-045-001-ml-envelope-cross-service-routing/scopes.md Scope 2
//   - scripts/commands/config.sh (the caller — atomic-promote wrapper)
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/smackerel/smackerel/internal/config"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the testable entry point. It returns the process exit code
// so tests can exercise the binary's contract without spawning a
// subprocess for every case.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("config-validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	envFile := fs.String("env-file", "", "path to env file to validate (KEY=VALUE per line)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *envFile == "" {
		fmt.Fprintln(stderr, "usage: config-validate --env-file=<path>")
		return 2
	}
	if err := loadEnvFile(*envFile); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "config-validate: %s OK\n", *envFile)
	return 0
}

// loadEnvFile reads KEY=VALUE pairs from path and applies each via
// os.Setenv. Outer double or single quotes are stripped. Lines that
// begin with '#' (after leading whitespace) and blank lines are
// skipped. Any other line that does not match KEY=VALUE is a
// fail-loud error (returned to the caller, which exits with code 2).
//
// This parser is deliberately narrow: it matches the shape that
// scripts/commands/config.sh emits today (one variable per line, no
// multi-line values, no shell substitutions — substitutions are
// resolved before the file is written by the heredoc-with-EOF in
// config.sh). It does not implement bash variable expansion, command
// substitution, or array syntax. If config.sh ever emits a richer
// shape, both writer and reader must change together.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Allow long lines (some env values like
	// ML_MODEL_MEMORY_PROFILES_JSON can be a few KiB of JSON).
	const maxLine = 1024 * 1024 // 1 MiB
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxLine)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		idx := strings.Index(raw, "=")
		if idx <= 0 {
			return fmt.Errorf("env file %s line %d: malformed (no KEY=VALUE form)", path, lineNum)
		}
		key := strings.TrimSpace(raw[:idx])
		val := raw[idx+1:]
		// Strip outer matched quotes (single or double). Mirrors the
		// emission shape config.sh uses for quoted values.
		if len(val) >= 2 {
			first, last := val[0], val[len(val)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if err := os.Setenv(key, val); err != nil {
			return fmt.Errorf("setenv %s: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}
	return nil
}
