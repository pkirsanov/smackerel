// Package preflight implements the local resource pre-flight guard for spec 099.
//
// Before each heavy ./smackerel.sh operation (build, up, and the live-stack
// test categories integration/e2e/e2e-ui/stress), the guard checks host
// available RAM (MemAvailable from /proc/meminfo) and available disk (on the
// repo filesystem) against SST-configured minimums and fails fast with an
// actionable message instead of letting a doomed run be OOM-killed (exit 137)
// or run the disk out minutes in.
//
// The package is split into a PURE core (Thresholds/Resources/Result,
// ParseThresholds, Evaluate, FormatReport, Run) that does no I/O and is fully
// unit-tested, and thin host-I/O helpers (LoadEnvFile, ReadMemAvailableMB,
// ReadDiskAvailableMB) that the cmd/preflight glue calls at runtime.
//
// Thresholds come from config/smackerel.yaml runtime.preflight.* via the
// generated env file (PREFLIGHT_MIN_AVAILABLE_RAM_MB / _DISK_GB). A missing,
// empty, non-numeric, or non-positive value fails loud naming the key — there
// is NO hidden default (Gate G028 / NO-DEFAULTS / fail-loud SST).
package preflight

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// Env var keys carried by the generated env file (config/generated/<env>.env),
// emitted by scripts/commands/config.sh from config/smackerel.yaml
// runtime.preflight.*. These are the ONLY env values the guard consumes.
const (
	EnvKeyMinRAMMB  = "PREFLIGHT_MIN_AVAILABLE_RAM_MB"
	EnvKeyMinDiskGB = "PREFLIGHT_MIN_AVAILABLE_DISK_GB"
	// OverrideEnvKey, when truthy, bypasses the gate with a loud WARNING.
	OverrideEnvKey = "SMACKEREL_PREFLIGHT_OVERRIDE"
)

// Thresholds are the SST-configured minimums. Both are required; the guard
// never supplies a default for either.
type Thresholds struct {
	MinAvailableRAMMB  int64
	MinAvailableDiskGB int64
}

// Resources are the observed host resources at check time.
type Resources struct {
	AvailableRAMMB  int64
	AvailableDiskMB int64
}

// Result is the outcome of comparing Resources against Thresholds.
type Result struct {
	OK        bool
	RAMShort  bool
	DiskShort bool
}

// ParseThresholds reads the two required keys from a key=value env map. A
// missing, empty, non-numeric, or non-positive value returns an error NAMING
// the offending key (Gate G028 / NO-DEFAULTS). No fallback is ever supplied.
func ParseThresholds(env map[string]string) (Thresholds, error) {
	ram, err := requirePositiveInt(env, EnvKeyMinRAMMB)
	if err != nil {
		return Thresholds{}, err
	}
	disk, err := requirePositiveInt(env, EnvKeyMinDiskGB)
	if err != nil {
		return Thresholds{}, err
	}
	return Thresholds{MinAvailableRAMMB: ram, MinAvailableDiskGB: disk}, nil
}

func requirePositiveInt(env map[string]string, key string) (int64, error) {
	raw, ok := env[key]
	if !ok || strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf(
			"required config key %s is missing or empty (NO-DEFAULTS / Gate G028): set runtime.preflight in config/smackerel.yaml and run ./smackerel.sh config generate",
			key,
		)
	}
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("required config key %s must be a positive integer, got %q", key, raw)
	}
	if v <= 0 {
		return 0, fmt.Errorf("required config key %s must be a positive integer, got %d", key, v)
	}
	return v, nil
}

// Evaluate compares observed resources against thresholds. It performs NO I/O.
func Evaluate(res Resources, th Thresholds) Result {
	ramShort := res.AvailableRAMMB < th.MinAvailableRAMMB
	diskShort := res.AvailableDiskMB < th.MinAvailableDiskGB*1024
	return Result{
		OK:        !ramShort && !diskShort,
		RAMShort:  ramShort,
		DiskShort: diskShort,
	}
}

// FormatReport renders the human-readable current-vs-required report plus
// actionable remediation. It interpolates ONLY the four numeric values (two
// observed, two threshold) — it NEVER interpolates any other env value, so it
// is structurally incapable of echoing a secret carried by the env map.
func FormatReport(res Resources, th Thresholds, result Result, overridden bool) string {
	var b strings.Builder

	status := "OK"
	if !result.OK {
		status = "BELOW THRESHOLD"
	}
	fmt.Fprintf(&b, "Smackerel pre-flight resource check: %s\n", status)
	fmt.Fprintf(&b, "  RAM  available: %d MB (required >= %d MB)%s\n",
		res.AvailableRAMMB, th.MinAvailableRAMMB, shortMark(result.RAMShort))
	fmt.Fprintf(&b, "  Disk available: %d MB / %.1f GB (required >= %d GB)%s\n",
		res.AvailableDiskMB, float64(res.AvailableDiskMB)/1024.0, th.MinAvailableDiskGB, shortMark(result.DiskShort))

	if !result.OK {
		b.WriteString("\nRemediation (free host resources before retrying the heavy operation):\n")
		b.WriteString("  - Stop idle Docker stacks you are not actively using.\n")
		b.WriteString("  - Stop Ollama if a local model is resident and not needed right now.\n")
		b.WriteString("  - Reclaim project Docker space:  ./smackerel.sh clean smart\n")
		b.WriteString("  - Override (proceed anyway, at your own risk):  SMACKEREL_PREFLIGHT_OVERRIDE=1\n")
	}

	if overridden {
		b.WriteString("\nWARNING: " + OverrideEnvKey + " is set \u2014 proceeding DESPITE the resource check." +
			" A heavy run may still be OOM-killed (exit 137) or fill the disk.\n")
	}

	return b.String()
}

func shortMark(short bool) string {
	if short {
		return "  <-- SHORT"
	}
	return ""
}

// Run is the full decision path: parse thresholds (fail-loud), evaluate, render
// the report, and compute the exit code. When overridden is true the exit code
// is forced to 0 (the report carries the WARNING). It performs NO host I/O —
// the caller supplies the observed Resources — so the whole decision is unit
// testable with synthetic inputs.
func Run(env map[string]string, res Resources, overridden bool) (report string, exitCode int, err error) {
	th, perr := ParseThresholds(env)
	if perr != nil {
		return "", 1, perr
	}
	result := Evaluate(res, th)
	report = FormatReport(res, th, result, overridden)
	exitCode = 0
	if !result.OK && !overridden {
		exitCode = 1
	}
	return report, exitCode, nil
}

// Truthy reports whether s is a recognized truthy flag value. Used for the
// override env var. An unset/empty/unrecognized value is false.
func Truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// LoadEnvFile parses a generated env file (key=value lines, # comments and
// blank lines ignored) into a map. Only the first '=' splits each line, so
// values containing '=' survive intact.
func LoadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open env file %s: %w", path, err)
	}
	defer f.Close()

	env := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := line[eq+1:]
		if key == "" {
			continue
		}
		env[key] = val
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read env file %s: %w", path, err)
	}
	return env, nil
}

// ReadMemAvailableMB returns host MemAvailable in MB by parsing /proc/meminfo.
func ReadMemAvailableMB() (int64, error) {
	return readMemAvailableMBFrom("/proc/meminfo")
}

// readMemAvailableMBFrom parses the MemAvailable line (reported in kB) from the
// given meminfo-format file and returns MB. Split out so a unit test can feed a
// synthetic file without touching the real /proc.
func readMemAvailableMBFrom(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("malformed MemAvailable line in %s: %q", path, line)
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse MemAvailable in %s: %w", path, err)
		}
		return kb / 1024, nil
	}
	if err := sc.Err(); err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	return 0, fmt.Errorf("MemAvailable not found in %s", path)
}

// ReadDiskAvailableMB returns the available disk space (MB) for unprivileged
// users on the filesystem backing path, via statfs. When path is a bind mount
// (e.g. /workspace inside the dockerized Go runner), statfs follows to the
// underlying host filesystem, so the value reflects the real host repo fs.
func ReadDiskAvailableMB(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	availBytes := int64(st.Bavail) * int64(st.Bsize)
	return availBytes / (1024 * 1024), nil
}
