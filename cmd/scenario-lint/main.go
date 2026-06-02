// scenario-lint validates scenario YAML files against every load-time
// rule from spec 037 design §2.2 (BS-009 / BS-010 / BS-011) and exits
// non-zero on any rejection.
//
// Intended to be wired into CI alongside ./smackerel.sh check. Usage:
//
//	scenario-lint <dir> [-glob "*.yaml"]
//
// The binary registers no tools by itself; callers that want
// allowlist references (BS-010) checked must run the linter inside a
// process that has the relevant tool packages imported. CI treats a
// missing tool reference as a load failure, which is the correct
// behavior — scenarios that refer to tools their owning package no
// longer registers should fail the build.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/smackerel/smackerel/internal/agent"
	_ "github.com/smackerel/smackerel/internal/agent/tools/microtools"
	_ "github.com/smackerel/smackerel/internal/agent/tools/notification"
	_ "github.com/smackerel/smackerel/internal/agent/tools/recipesearch"
	_ "github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	_ "github.com/smackerel/smackerel/internal/agent/tools/weather"
	"github.com/smackerel/smackerel/internal/assistant"

	// Spec 064 SCOPE-12 — substrate-bridge tool open_knowledge_invoke
	// registers itself at package-init time so config/prompt_contracts/
	// open_knowledge.yaml passes loader allow_tools validation here.
	_ "github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	_ "github.com/smackerel/smackerel/internal/recommendation/tools"
)

func main() {
	if code := run(os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

// run is the testable entry point. It returns the process exit code
// (0 = clean, 1 = rejection, 2 = usage error).
func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("scenario-lint", flag.ContinueOnError)
	fs.SetOutput(stderr)
	glob := fs.String("glob", "", "filename glob (default: *.yaml + *.yml)")
	assistantManifest := fs.String("assistant-manifest", "",
		"optional path to the spec 061 assistant skills manifest (config/assistant/scenarios.yaml); "+
			"when set, also runs design §7.2 rule #6 (every manifest scenario id MUST have a loadable YAML)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: scenario-lint <dir> [-glob PATTERN] [-assistant-manifest PATH]")
		return 2
	}
	dir := fs.Arg(0)
	abs, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(stderr, "resolve %s: %v\n", dir, err)
		return 2
	}

	registered, rejected, fatal := agent.DefaultLoader().Load(abs, *glob)

	for _, r := range rejected {
		fmt.Fprintf(stderr, "REJECT %s: %s\n", r.Path, r.Message)
	}
	if fatal != nil {
		fmt.Fprintf(stderr, "FATAL %v\n", fatal)
	}
	fmt.Fprintf(stdout, "scenarios registered: %d, rejected: %d\n", len(registered), len(rejected))

	if fatal != nil || len(rejected) > 0 {
		return 1
	}

	// Optional spec 061 design §7.2 rule #6 — when the operator passes
	// -assistant-manifest, also verify every manifest scenario id has a
	// loadable YAML in the same directory the loader just scanned.
	if *assistantManifest != "" {
		manifestAbs, err := filepath.Abs(*assistantManifest)
		if err != nil {
			fmt.Fprintf(stderr, "resolve %s: %v\n", *assistantManifest, err)
			return 2
		}
		// Linter-time resolver: every enable_sst_key is treated as
		// enabled so rule #6 covers all manifest ids regardless of the
		// runtime SST snapshot.
		acceptAll := func(string) (bool, bool) { return true, true }
		if err := assistant.ValidateScenariosPresent(manifestAbs, abs, acceptAll); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		fmt.Fprintf(stdout, "assistant manifest %s — rule #6 OK\n", manifestAbs)
	}
	return 0
}
