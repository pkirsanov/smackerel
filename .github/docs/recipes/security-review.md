# Recipe: Security Review

> *"Safety... always ON."*

---

## The Situation

You need to check for security vulnerabilities before shipping.

## The Command

```
/bubbles.security  review security for 042-catalog-assistant
```

## What Gets Checked

- OWASP Top 10 vulnerabilities
- SQL injection / XSS / command injection
- Authentication and authorization gaps
- Cryptographic issues
- Insecure dependencies
- Secrets in code
- SSRF/CSRF risks
- Input validation gaps

## G028 Privileged Scan 2B Review

Run the authoritative reality scan through the source repository CLI:

```bash
bash bubbles/scripts/cli.sh scan <classified-work-path> --verbose
```

Do not recommend a raw `implementation-reality-scan.sh` invocation. That path
uses `compat-reexec` and cannot claim pre-boundary cleanliness.

Review the `privileged-native-supervision-v2` contract against the exact clean,
immutable candidate:

- `SEC-R1`: Confirm both canonical callers enter `/usr/bin/env -i` with `BUBBLES_SECURITY_ENTRY_MODE=direct` and `/bin/bash -p` before module sourcing.
- Test hostile startup functions and variables for `SEC-R1`.
- `SEC-R2`: Authenticate fixed `/usr/bin/perl` as `root-protected-perl-supervisor-v1`.
- Confirm Perl owns one direct worker and signals it only while unreaped.
- Confirm Perl calls `waitpid` and emits one authentic `BPS1` record after reap.
- `HAR-R1`: Confirm Bash holds only a supervisor wait handle. Reject Bash worker or watchdog PID signaling, process-group cleanup, and stale-PID reasoning.
- `HAR-R2`: Confirm the worker closes the supervisor control descriptor before `exec`. Treat worker text, pipe EOF, and descriptor-holding descendants as data, never completion authority.
- `HAR-R3`: Bind `BSEC1`, `BPS1`, `root-protected-native-python-v1`, `PYSEC1`, `PYMOD1`, and `SCS1` to one exact candidate.
- Verify the fixed 30-second wall, two-second grace, output bounds, signal owner, timeout bit, worker kind, byte counts, and numeric status semantics.
- Require missing or untrusted Perl to fail closed as `SUPERVISOR_UNAVAILABLE` or `SUPERVISOR_UNTRUSTED`.
- Require a root-protected fixed `/usr/bin/perl` and an authenticated Python toolchain for remediation.
- Reject PATH-selected Perl, Bash or Python supervision, external-timeout substitution, fallback paths, and bypasses.
- Reject recursive descendant-containment claims. The supervisor owns and reaps only its direct worker.
- Reject diagnostics that replay raw worker output, environment values, or PIDs. Accept only bounded status, closed enums, and protocol identities.

## After Security Scan

Fix findings, then validate:

```
/bubbles.workflow  042-catalog-assistant mode: full-delivery
```
