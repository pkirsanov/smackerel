---
applyTo: "**"
---

# Terminal Discipline Policy (NON-NEGOTIABLE)

## 1. No Piping/Redirecting Output Into Files (ABSOLUTE)

**FORBIDDEN:** Using shell pipes or redirects to write files.

```bash
# ❌ FORBIDDEN
echo "content" > /path/to/file.txt
cat source.txt > dest.txt
command | tee output.log
command > output.txt 2>&1
```

**REQUIRED:** Use the dedicated file creation/editing tools provided by the IDE.

---

## 2. No Truncating Command Output (ABSOLUTE)

**FORBIDDEN:** Filtering, truncating, or limiting command output with pipes.

```bash
# ❌ FORBIDDEN
command | head -20
command | tail -50
command | grep "pattern" | head
```

**REQUIRED:** Always capture and display the FULL unfiltered output.

---

## 3. Always Use Repo CLI — No Ad-Hoc Commands (ABSOLUTE)

**FORBIDDEN:** Running build, test, lint, deploy, or service management commands directly.

```bash
# ❌ FORBIDDEN — bypassing repo CLI
cargo build
npm test
go test ./...
python -m pytest
docker compose up
```

**REQUIRED:** Use `./smackerel.sh` for ALL build, test, lint, deploy, and service operations.

```bash
# ✅ REQUIRED
./smackerel.sh build
./smackerel.sh test
./smackerel.sh lint
```

**Exception:** Read-only inspection commands that don't build, test, or modify state are allowed:
- `ls`, `cat`, `find`, `grep`, `wc` for exploring the filesystem
- `git log`, `git diff`, `git status` for version control inspection
- `docker ps`, `docker logs` for observing running containers
- `curl --max-time 5` for quick health checks (always with timeout)

---

## Summary Table

| Category | FORBIDDEN | REQUIRED |
|----------|-----------|----------|
| **File writes** | `>`, `>>`, `tee`, heredoc-to-file, pipe-to-file | IDE file tools (create_file, replace_string_in_file) |
| **Output filtering** | `head`, `tail`, `awk 'NR<=N'`, `sed -n`, pipe-to-grep on commands | Full unfiltered output from every command |
| **Build/test/lint** | Direct tool invocation | `./smackerel.sh <command>` exclusively |

**Violations of this policy are blocking issues.**
