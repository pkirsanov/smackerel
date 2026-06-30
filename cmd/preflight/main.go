// Command preflight is the local resource pre-flight guard entrypoint for
// spec 099. It reads host available RAM (MemAvailable from /proc/meminfo) and
// available disk (on the repo filesystem) and compares them against the
// SST-configured minimums carried by the generated env file. It exits 0 when
// the host meets the minimums and 1 when it is below, printing a
// current-vs-required report plus actionable remediation on a shortfall.
//
// SMACKEREL_PREFLIGHT_OVERRIDE=1 bypasses the gate (exit 0) with a loud
// WARNING. All decision logic lives in internal/preflight (pure, unit-tested);
// this file is thin glue that performs the host I/O.
//
// Usage:
//
//	go run ./cmd/preflight --env <dev|test> --repo-root <abs-path> --profile <heavy|light>
//
// All three flags are REQUIRED — there is no default (Gate G028 / NO-DEFAULTS).
// The env file path is derived as <repo-root>/config/generated/<env>.env and
// the disk check targets <repo-root>. The profile selects which SST threshold
// pair is enforced: heavy (build/up/full test lanes) or light (the stores-only
// integration-light lane).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/smackerel/smackerel/internal/preflight"
)

func main() {
	envName := flag.String("env", "", "target environment name (dev|test); required")
	repoRoot := flag.String("repo-root", "", "absolute path to the repo root; required")
	profileName := flag.String("profile", "", "threshold profile (heavy|light); required")
	flag.Parse()

	if *envName == "" {
		fatalf("--env is required (no default; Gate G028 / NO-DEFAULTS)")
	}
	if *repoRoot == "" {
		fatalf("--repo-root is required (no default; Gate G028 / NO-DEFAULTS)")
	}
	if *profileName == "" {
		fatalf("--profile is required (heavy|light; no default; Gate G028 / NO-DEFAULTS)")
	}
	profile, err := preflight.ParseProfile(*profileName)
	if err != nil {
		fatalf("%v", err)
	}

	envFile := filepath.Join(*repoRoot, "config", "generated", *envName+".env")
	env, err := preflight.LoadEnvFile(envFile)
	if err != nil {
		fatalf("%v", err)
	}

	ramMB, err := preflight.ReadMemAvailableMB()
	if err != nil {
		fatalf("read host RAM: %v", err)
	}
	diskMB, err := preflight.ReadDiskAvailableMB(*repoRoot)
	if err != nil {
		fatalf("read host disk: %v", err)
	}

	res := preflight.Resources{AvailableRAMMB: ramMB, AvailableDiskMB: diskMB}
	overridden := preflight.Truthy(os.Getenv(preflight.OverrideEnvKey))

	report, exitCode, err := preflight.RunForProfile(env, res, overridden, profile)
	if err != nil {
		// Fail-loud on a missing/invalid SST threshold key.
		fatalf("%v", err)
	}

	fmt.Print(report)
	os.Exit(exitCode)
}

func fatalf(format string, args ...any) {
	fmt.Fprintln(os.Stderr, "ERROR: smackerel pre-flight: "+fmt.Sprintf(format, args...))
	os.Exit(1)
}
