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
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: scenario-lint <dir> [-glob PATTERN]")
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
	return 0
}
