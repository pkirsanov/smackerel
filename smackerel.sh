#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/scripts/lib/runtime.sh"

TARGET_ENV="dev"
NO_CACHE=false
FORMAT_CHECK=false
DOWN_VOLUMES=false

usage() {
  cat <<'EOF'
Usage: ./smackerel.sh [--env dev|test] <command> [options]

Commands:
  config generate             Generate config/generated/<env>.env from config/smackerel.yaml
  build [--no-cache]          Build docker images for the current environment
  backup                      Create a compressed pg_dump backup in backups/
  check                       Validate generated config and docker-compose wiring
  lint                        Run Go vet, Python ruff, and web asset validation
  format [--check]            Format Go and Python files, or check formatting
  package extension           Package browser extension for Chrome and Firefox distribution
  test unit [--go|--python]   Run unit tests
  test integration            Run live-stack integration validation
  test e2e [--go-run <regex>] Run E2E tests; optionally run only matching Go E2E tests
  test stress                 Run live-stack stress smoke test
  up                          Start the stack for the current environment
  down [--volumes]            Stop the stack; optionally remove named volumes
  status                      Show docker status and health endpoint output
  logs [service]              Stream docker compose logs
  clean status                Show project-scoped cleanup state
  clean measure               Show docker disk usage
  clean smart                 Stop the current stack without deleting persistent volumes
  clean full                  Stop the current stack and remove project-scoped volumes
EOF
}

require_docker() {
  command -v docker >/dev/null 2>&1 || {
    echo "docker is required" >&2
    exit 1
  }
}

run_go_tooling() {
  local script_path="$1"
  shift || true
  docker run --rm \
    -v "$SCRIPT_DIR:/workspace" \
    -v smackerel-gomod-cache:/go/pkg/mod \
    -v smackerel-gobuild-cache:/root/.cache/go-build \
    -w /workspace \
    golang:1.24.3-bookworm bash "$script_path" "$@"
}

run_python_tooling() {
  local script_path="$1"
  shift || true
  docker run --rm \
    -v "$SCRIPT_DIR:/workspace" \
    -v smackerel-pip-cache:/root/.cache/pip \
    -w /workspace \
    python:3.12-slim bash "$script_path" "$@"
}

positional=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --env)
      if [[ $# -lt 2 || -z "${2:-}" ]]; then
        echo "ERROR: --env requires dev or test" >&2
        exit 1
      fi
      TARGET_ENV="$2"
      shift 2
      ;;
    --env=*)
      TARGET_ENV="${1#*=}"
      shift
      ;;
    --no-cache)
      NO_CACHE=true
      shift
      ;;
    --check)
      FORMAT_CHECK=true
      shift
      ;;
    --volumes)
      DOWN_VOLUMES=true
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      positional+=("$1")
      shift
      ;;
  esac
done
set -- "${positional[@]}"

smackerel_require_env_value() {
  local env_file="$1"
  local key="$2"
  local value

  value="$(smackerel_env_value "$env_file" "$key")"
  if [[ -z "$value" ]]; then
    echo "ERROR: $key missing from $env_file" >&2
    exit 1
  fi
  printf '%s\n' "$value"
}

smackerel_assert_host_ports_free() {
  local env_file="$1"
  local host_bind_address
  local compose_project
  local enable_ollama
  local port_key
  local port_value
  local wait_timeout_s=60
  local wait_interval_s=2
  local port_specs=()

  host_bind_address="$(smackerel_require_env_value "$env_file" "HOST_BIND_ADDRESS")"
  compose_project="$(smackerel_require_env_value "$env_file" "COMPOSE_PROJECT")"
  enable_ollama="$(smackerel_require_env_value "$env_file" "ENABLE_OLLAMA")"

  for port_key in CORE_HOST_PORT ML_HOST_PORT POSTGRES_HOST_PORT NATS_CLIENT_HOST_PORT NATS_MONITOR_HOST_PORT; do
    port_value="$(smackerel_require_env_value "$env_file" "$port_key")"
    port_specs+=("${port_key}=${port_value}")
  done

  if smackerel_is_truthy "$enable_ollama"; then
    port_value="$(smackerel_require_env_value "$env_file" "OLLAMA_HOST_PORT")"
    port_specs+=("OLLAMA_HOST_PORT=${port_value}")
  fi

  python3 - "$host_bind_address" "$compose_project" "$wait_timeout_s" "$wait_interval_s" "${port_specs[@]}" <<'PY'
import os
import socket
import subprocess
import sys
import time

bind_address = sys.argv[1]
compose_project = sys.argv[2]
wait_timeout_s = int(sys.argv[3])
wait_interval_s = float(sys.argv[4])
port_specs = sys.argv[5:]
ports = []


def conflict_message(key, raw_port, exc):
    return f"{key}={raw_port} on {bind_address}:{raw_port}: {exc}"


def parse_ports():
    parsed = []
    parse_errors = []
    for port_spec in port_specs:
        key, raw_port = port_spec.split("=", 1)
        try:
            port = int(raw_port)
        except ValueError:
            parse_errors.append({
                "key": key,
                "port": raw_port,
                "message": f"{key}={raw_port} is not a numeric port",
                "owners": [],
                "external_owners": [],
            })
            continue
        parsed.append((key, raw_port, port))
    return parsed, parse_errors


def bind_conflicts():
    conflicts = []
    for key, raw_port, port in ports:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            try:
                sock.bind((bind_address, port))
            except OSError as exc:
                conflicts.append({
                    "key": key,
                    "port": port,
                    "raw_port": raw_port,
                    "message": conflict_message(key, raw_port, exc),
                    "owners": [],
                    "external_owners": [],
                })
    return conflicts


def listen_inodes_by_port():
    inodes = {}
    for proc_file in ("/proc/net/tcp", "/proc/net/tcp6"):
        try:
            with open(proc_file, "r", encoding="utf-8") as handle:
                rows = handle.readlines()[1:]
        except OSError:
            continue
        for row in rows:
            fields = row.split()
            if len(fields) < 10 or fields[3] != "0A":
                continue
            local = fields[1]
            if ":" not in local:
                continue
            _, raw_port = local.rsplit(":", 1)
            try:
                port = int(raw_port, 16)
            except ValueError:
                continue
            inodes.setdefault(port, set()).add(fields[9])
    return inodes


def process_owners_for_ports(target_ports):
    inodes = listen_inodes_by_port()
    wanted = {inode: port for port in target_ports for inode in inodes.get(port, set())}
    owners = {port: [] for port in target_ports}
    if not wanted:
        return owners

    for pid in os.listdir("/proc"):
        if not pid.isdigit():
            continue
        fd_dir = f"/proc/{pid}/fd"
        try:
            fd_names = os.listdir(fd_dir)
        except OSError:
            continue
        matched_ports = set()
        for fd_name in fd_names:
            try:
                target = os.readlink(os.path.join(fd_dir, fd_name))
            except OSError:
                continue
            if not target.startswith("socket:[") or not target.endswith("]"):
                continue
            inode = target[8:-1]
            port = wanted.get(inode)
            if port is not None:
                matched_ports.add(port)
        if not matched_ports:
            continue

        cmdline = ""
        try:
            with open(f"/proc/{pid}/cmdline", "rb") as handle:
                cmdline = handle.read().replace(b"\x00", b" ").decode("utf-8", "replace").strip()
        except OSError:
            pass
        if not cmdline:
            try:
                with open(f"/proc/{pid}/comm", "r", encoding="utf-8") as handle:
                    cmdline = handle.read().strip()
            except OSError:
                cmdline = "<unavailable>"

        for port in matched_ports:
            owners[port].append(f"process pid={pid} cmd={cmdline}")
    return owners


def docker_owners_for_ports(target_ports):
    owners = {port: [] for port in target_ports}
    format_arg = '{{.ID}}\t{{.Names}}\t{{.Status}}\t{{.Ports}}\t{{.Label "com.docker.compose.project"}}'
    try:
        result = subprocess.run(
            ["docker", "ps", "-a", "--format", format_arg],
            check=False,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
    except OSError as exc:
        for port in target_ports:
            owners[port].append(f"docker owner lookup unavailable: {exc}")
        return owners

    if result.returncode != 0:
        message = result.stderr.strip() or f"docker ps exited {result.returncode}"
        for port in target_ports:
            owners[port].append(f"docker owner lookup failed: {message}")
        return owners

    for row in result.stdout.splitlines():
        parts = row.split("\t")
        if len(parts) != 5:
            continue
        container_id, names, status, published_ports, project = parts
        for port in target_ports:
            if f":{port}->" in published_ports or f"0.0.0.0:{port}->" in published_ports or f"127.0.0.1:{port}->" in published_ports:
                owners[port].append(
                    f"docker container id={container_id} name={names} status={status} compose_project={project or '<none>'} ports={published_ports}"
                )
    return owners


def attach_owners(conflicts):
    target_ports = [conflict["port"] for conflict in conflicts if isinstance(conflict.get("port"), int)]
    process_owners = process_owners_for_ports(target_ports)
    docker_owners = docker_owners_for_ports(target_ports)
    for conflict in conflicts:
        port = conflict.get("port")
        if not isinstance(port, int):
            continue
        owners = docker_owners.get(port, []) + process_owners.get(port, [])
        if not owners:
            owners = ["no process owner visible from /proc or docker ps"]
        conflict["owners"] = owners
        external = []
        for owner in owners:
            if owner.startswith("docker container"):
                if f"compose_project={compose_project}" not in owner:
                    external.append(owner)
                continue
            if "docker-proxy" in owner or "rootlesskit" in owner:
                continue
            if "owner lookup" in owner or "no process owner visible" in owner:
                continue
            external.append(owner)
        conflict["external_owners"] = external
    return conflicts


def print_conflicts(header, conflicts):
    print(header, file=sys.stderr)
    print("Unavailable test port(s):", file=sys.stderr)
    for conflict in conflicts:
        print(f"  - {conflict['message']}", file=sys.stderr)
        for owner in conflict.get("owners", []):
            print(f"    owner: {owner}", file=sys.stderr)


ports, initial_errors = parse_ports()
if initial_errors:
    print_conflicts("ERROR: Smackerel host port preflight failed due to invalid generated port config.", initial_errors)
    sys.exit(1)

started_at = time.monotonic()
reported_wait = False

while True:
    conflicts = bind_conflicts()
    if not conflicts:
        if reported_wait:
            elapsed = time.monotonic() - started_at
            print(f"Configured test host ports became free after {elapsed:.1f}s.")
        sys.exit(0)

    attach_owners(conflicts)
    external_conflicts = [conflict for conflict in conflicts if conflict.get("external_owners")]
    if external_conflicts:
        print_conflicts(
            "ERROR: Smackerel host port preflight found non-Smackerel listener(s).",
            external_conflicts,
        )
        print("Stop the reported non-Smackerel listener before starting the disposable test stack.", file=sys.stderr)
        sys.exit(1)

    elapsed = time.monotonic() - started_at
    if elapsed >= wait_timeout_s:
        print_conflicts(
            f"ERROR: Smackerel host port preflight timed out after {wait_timeout_s}s waiting for project-scoped port release.",
            conflicts,
        )
        print("Project-scoped cleanup completed, but one or more configured test ports stayed bound.", file=sys.stderr)
        sys.exit(1)

    if not reported_wait:
        print(f"Waiting for configured test host ports to be released after project-scoped cleanup (timeout {wait_timeout_s}s)...")
        reported_wait = True
    time.sleep(wait_interval_s)
PY
}

smackerel_prepare_test_stack_for_up() {
  local env_file="$1"

  echo "Preparing disposable test stack..."
  smackerel_compose test down --timeout 60 --remove-orphans
  smackerel_assert_host_ports_free "$env_file"
}

COMMAND="${1:-help}"
shift || true

case "$COMMAND" in
  config)
    SUBCOMMAND="${1:-}"
    case "$SUBCOMMAND" in
      generate)
        smackerel_generate_config "$TARGET_ENV"
        ;;
      *)
        usage
        exit 1
        ;;
    esac
    ;;
  build)
    require_docker
    smackerel_generate_config "$TARGET_ENV" >/dev/null
    build_args=(build)
    if [[ "$NO_CACHE" == true ]]; then
      build_args+=(--no-cache)
    fi
    if [[ "$TARGET_ENV" == "test" ]]; then
      build_args+=(--build-arg GO_BUILD_TAGS=e2e)
    fi
    smackerel_compose "$TARGET_ENV" "${build_args[@]}"
    ;;
  backup)
    require_docker
    smackerel_generate_config "$TARGET_ENV" >/dev/null
    bash "$SCRIPT_DIR/scripts/commands/backup.sh" --env "$TARGET_ENV"
    ;;
  check)
    require_docker
    smackerel_generate_config "$TARGET_ENV" >/dev/null
    smackerel_compose "$TARGET_ENV" config -q
    # Config drift check — verify generated env files match SST
    if ! git diff --quiet -- config/generated/ 2>/dev/null; then
        echo "ERROR: Config drift detected — generated files differ from SST. Run: ./smackerel.sh config generate"
        exit 1
    fi
    echo "Config is in sync with SST"
    # env_file drift guard — verify core/ml services use env_file, not individual SST vars
    if ! grep -q 'env_file:' docker-compose.yml; then
        echo "ERROR: docker-compose.yml missing env_file: directive — SST vars must flow through config/generated/dev.env"
        exit 1
    fi
    # Build comprehensive SST var list from the generated env file, then check that
    # none appear as individual declarations in the core/ml environment blocks.
    # Only check services that use env_file (core and ml); postgres/nats keep their own blocks.
    # Allowed overrides in core/ml (container-path remaps, not SST-managed):
    ALLOWED_OVERRIDES="^(PORT|BOOKMARKS_IMPORT_DIR|MAPS_IMPORT_DIR|BROWSER_HISTORY_PATH|TWITTER_ARCHIVE_DIR|PROMPT_CONTRACTS_DIR|AGENT_SCENARIO_DIR|LOG_LEVEL)$"
    env_file="$(smackerel_env_file "$TARGET_ENV")"
    # Extract the smackerel-core and smackerel-ml service blocks (indented 4+ spaces under the service)
    core_ml_env="$(awk '
        /^  smackerel-core:|^  smackerel-ml:/ { in_svc=1; next }
        /^  [a-z]/ && in_svc { in_svc=0 }
        in_svc && /^\s+environment:/ { in_env=1; next }
        in_svc && in_env && /^      [A-Z_]+:/ { print; next }
        in_svc && in_env && /^    [a-z]/ { in_env=0 }
    ' docker-compose.yml)"
    drift_violations=""
    while IFS='=' read -r key _; do
        # Skip comments and empty lines
        [[ "$key" =~ ^#.*$ || -z "$key" ]] && continue
        # Skip allowed container-path overrides
        if echo "$key" | grep -qE "$ALLOWED_OVERRIDES"; then
            continue
        fi
        # Check if this SST var appears in core/ml environment blocks
        if echo "$core_ml_env" | grep -qE "^\s+${key}:"; then
            drift_violations="${drift_violations}  - ${key}\n"
        fi
    done < "$env_file"
    if [[ -n "$drift_violations" ]]; then
        echo "ERROR: docker-compose.yml core/ml services contain individual SST-managed env declarations — use env_file: instead"
        printf "Offending vars:\n%b" "$drift_violations"
        exit 1
    fi
    echo "env_file drift guard: OK"
    # Spec 037 Scope 10 — scenario-lint guards every scenario YAML
    # against the load-time rules (BS-009 / BS-010 / BS-011) before any
    # runtime can pick them up.
    run_go_tooling /workspace/scripts/runtime/scenario-lint.sh "config/generated/${TARGET_ENV}.env"
    echo "scenario-lint: OK"
    ;;
  lint)
    run_go_tooling /workspace/scripts/runtime/go-lint.sh
    run_python_tooling /workspace/scripts/runtime/python-lint.sh
    run_python_tooling /workspace/scripts/runtime/web-validate.sh
    ;;
  package)
    SUBCOMMAND="${1:-}"
    case "$SUBCOMMAND" in
      extension)
        bash "$SCRIPT_DIR/scripts/commands/package-extension.sh"
        ;;
      *)
        echo "Unknown package target: $SUBCOMMAND" >&2
        echo "Available: extension" >&2
        exit 1
        ;;
    esac
    ;;
  format)
    if [[ "$FORMAT_CHECK" == true ]]; then
      run_go_tooling /workspace/scripts/runtime/go-format.sh --check
      run_python_tooling /workspace/scripts/runtime/python-format.sh --check
    else
      run_go_tooling /workspace/scripts/runtime/go-format.sh
      run_python_tooling /workspace/scripts/runtime/python-format.sh
    fi
    ;;
  test)
    SUBCOMMAND="${1:-}"
    shift || true
    case "$SUBCOMMAND" in
      unit)
        if [[ "${1:-}" == "--go" ]]; then
          run_go_tooling /workspace/scripts/runtime/go-unit.sh
        elif [[ "${1:-}" == "--python" ]]; then
          run_python_tooling /workspace/scripts/runtime/python-unit.sh
        else
          run_go_tooling /workspace/scripts/runtime/go-unit.sh
          run_python_tooling /workspace/scripts/runtime/python-unit.sh
        fi
        ;;
      integration)
        require_docker
        smackerel_generate_config test >/dev/null
        env_file="$(smackerel_require_env_file test)"
        pg_host_port="$(smackerel_env_value "$env_file" "POSTGRES_HOST_PORT")"
        nats_host_port="$(smackerel_env_value "$env_file" "NATS_CLIENT_HOST_PORT")"
        auth_token="$(smackerel_env_value "$env_file" "SMACKEREL_AUTH_TOKEN")"
        pg_user="$(smackerel_env_value "$env_file" "POSTGRES_USER")"
        pg_pass="$(smackerel_env_value "$env_file" "POSTGRES_PASSWORD")"
        pg_db="$(smackerel_env_value "$env_file" "POSTGRES_DB")"

        # Spec 037 Scope 10 — orchestrator owns the test-stack
        # lifecycle so the Go integration runner sees a live stack.
        # KEEP_STACK_UP=1 is explicit because the trap below owns final
        # teardown regardless of test outcome.
        integration_cleanup() {
          timeout 60 "$SCRIPT_DIR/smackerel.sh" --env test down --volumes >/dev/null 2>&1 || true
        }
        trap integration_cleanup EXIT

        # Run shell-based health probe (brings stack up + asserts health)
        timeout 300 env KEEP_STACK_UP=1 bash "$SCRIPT_DIR/tests/integration/test_runtime_health.sh"

        # Run Go integration tests against the live test stack
        docker run --rm \
          --network host \
          -v "$SCRIPT_DIR:/workspace" \
          -v smackerel-gomod-cache:/go/pkg/mod \
          -v smackerel-gobuild-cache:/root/.cache/go-build \
          -w /workspace \
          -e "DATABASE_URL=postgres://${pg_user}:${pg_pass}@127.0.0.1:${pg_host_port}/${pg_db}?sslmode=disable" \
          -e "POSTGRES_URL=postgres://${pg_user}:${pg_pass}@127.0.0.1:${pg_host_port}/${pg_db}?sslmode=disable" \
          -e "NATS_URL=nats://${auth_token}@127.0.0.1:${nats_host_port}" \
          -e "SMACKEREL_AUTH_TOKEN=${auth_token}" \
          golang:1.24.3-bookworm bash /workspace/scripts/runtime/go-integration.sh
        ;;
      e2e)
        GO_E2E_RUN_SELECTOR=""
        while [[ $# -gt 0 ]]; do
          case "$1" in
            --go-run)
              if [[ $# -lt 2 ]]; then
                echo "ERROR: --go-run requires a non-empty regex" >&2
                exit 1
              fi
              if [[ -z "$2" ]]; then
                echo "ERROR: --go-run requires a non-empty regex" >&2
                exit 1
              fi
              GO_E2E_RUN_SELECTOR="$2"
              shift 2
              ;;
            --go-run=*)
              GO_E2E_RUN_SELECTOR="${1#*=}"
              if [[ -z "$GO_E2E_RUN_SELECTOR" ]]; then
                echo "ERROR: --go-run requires a non-empty regex" >&2
                exit 1
              fi
              shift
              ;;
            *)
              echo "Unknown test e2e option: $1" >&2
              usage
              exit 1
              ;;
          esac
        done

        e2e_child_pid=""
        e2e_cleanup_ran=0
        # Docker network removal can legitimately run past 60s on busy hosts;
        # keep the wrapper bounded while allowing slow successful teardown.
        E2E_STACK_DOWN_TIMEOUT_S=180
        E2E_STACK_DOWN_SLOW_WARN_S=60

        e2e_stop_child() {
          if [[ -n "${e2e_child_pid:-}" ]] && kill -0 "$e2e_child_pid" 2>/dev/null; then
            kill -TERM "-$e2e_child_pid" 2>/dev/null || kill -TERM "$e2e_child_pid" 2>/dev/null || true
            wait "$e2e_child_pid" 2>/dev/null || true
          fi
          e2e_child_pid=""
        }

        e2e_print_test_stack_state() {
          local compose_project

          compose_project="$(smackerel_compose_project test)"
          echo "E2E test stack diagnostics for compose project ${compose_project}:" >&2
          docker ps -a --filter "label=com.docker.compose.project=${compose_project}" >&2 || true
          docker network ls --filter "label=com.docker.compose.project=${compose_project}" >&2 || true
          docker volume ls --filter "label=com.docker.compose.project=${compose_project}" >&2 || true
        }

        e2e_cleanup() {
          local cleanup_status=0

          if [[ "${e2e_cleanup_ran:-0}" == "1" ]]; then
            return 0
          fi
          e2e_cleanup_ran=1
          e2e_stop_child
          e2e_down_test_stack "exit cleanup" || cleanup_status=$?
          return "$cleanup_status"
        }

        e2e_cleanup_trap() {
          local status=$?
          local cleanup_status=0

          e2e_cleanup || cleanup_status=$?
          if [[ "$status" -eq 0 && "$cleanup_status" -ne 0 ]]; then
            exit "$cleanup_status"
          fi
          exit "$status"
        }

        trap e2e_cleanup_trap EXIT
        trap 'e2e_cleanup || true; exit 143' INT TERM

        e2e_run_child() {
          if command -v setsid >/dev/null 2>&1; then
            setsid --wait "$@" &
          else
            "$@" &
          fi
          e2e_child_pid=$!
          set +e
          wait "$e2e_child_pid"
          local status=$?
          e2e_child_pid=""
          return "$status"
        }

        e2e_down_test_stack() {
          local phase="$1"
          local started_at finished_at duration status

          started_at="$(date +%s)"
          echo "Running project-scoped test stack teardown (${phase}, timeout ${E2E_STACK_DOWN_TIMEOUT_S}s)..."
          set +e
          e2e_run_child timeout "$E2E_STACK_DOWN_TIMEOUT_S" "$SCRIPT_DIR/smackerel.sh" --env test down --volumes
          status=$?
          set -e
          finished_at="$(date +%s)"
          duration=$((finished_at - started_at))

          if [[ "$status" -eq 0 ]]; then
            if (( duration > E2E_STACK_DOWN_SLOW_WARN_S )); then
              echo "Test stack teardown completed in ${duration}s; slow but within the ${E2E_STACK_DOWN_TIMEOUT_S}s budget."
            fi
            return 0
          fi

          echo "ERROR: project-scoped test stack teardown failed during ${phase} after ${duration}s (exit ${status}, timeout ${E2E_STACK_DOWN_TIMEOUT_S}s)." >&2
          e2e_print_test_stack_state
          return "$status"
        }

        e2e_shell_results=()
        e2e_shell_failures=0
        e2e_overall_status=0

        e2e_record_shell_result() {
          local test_name="$1"
          local status="$2"

          if [[ "$status" -eq 0 ]]; then
            e2e_shell_results+=("PASS: ${test_name}")
            return 0
          fi

          e2e_shell_results+=("FAIL: ${test_name} (exit=${status})")
          e2e_shell_failures=$((e2e_shell_failures + 1))
          if [[ "$e2e_overall_status" -eq 0 ]]; then
            e2e_overall_status="$status"
          fi
        }

        e2e_run_shell_test() {
          local test_name="$1"
          local status
          shift

          set +e
          e2e_run_child "$@"
          status=$?
          set -e
          e2e_record_shell_result "$test_name" "$status"
          return 0
        }

        e2e_print_shell_summary() {
          local total passed result

          total=${#e2e_shell_results[@]}
          if [[ "$total" -eq 0 ]]; then
            return 0
          fi

          passed=$((total - e2e_shell_failures))
          echo ""
          echo "========================================="
          echo "  Shell E2E Test Results"
          echo "========================================="
          for result in "${e2e_shell_results[@]}"; do
            echo "  $result"
          done
          echo ""
          echo "  Total:  $total"
          echo "  Passed: $passed"
          echo "  Failed: $e2e_shell_failures"
          echo "========================================="
          echo ""
        }

        if [[ -z "$GO_E2E_RUN_SELECTOR" ]]; then
          # Lifecycle E2E scripts intentionally own their own stack boot,
          # restart, and teardown semantics.
          e2e_lifecycle_scripts=(
            test_compose_start.sh
            test_persistence.sh
            test_postgres_readiness_gate.sh
            test_config_fail.sh
          )
          for e2e_script in "${e2e_lifecycle_scripts[@]}"; do
            echo "Running isolated lifecycle shell E2E: $e2e_script"
            e2e_run_shell_test "$e2e_script" timeout 300 bash "$SCRIPT_DIR/tests/e2e/$e2e_script"
          done

          # Shared shell E2E scripts use one parent-owned disposable test
          # stack. Individual scripts run with E2E_STACK_MANAGED=1 so they
          # cannot boot, tear down, or leak fixed host-port listeners.
          e2e_shared_scripts=(
            # Scope 02: Processing pipeline
            test_capture_pipeline.sh
            test_voice_pipeline.sh
            test_llm_failure_e2e.sh
            # Scope 03: Active capture API
            test_capture_api.sh
            test_capture_errors.sh
            test_voice_capture_api.sh
            # Scope 04: Knowledge graph
            test_knowledge_graph.sh
            test_graph_entities.sh
            # Scope 05: Semantic search
            test_search.sh
            test_search_filters.sh
            test_search_empty.sh
            # Scope 06: Telegram bot
            test_telegram.sh
            test_telegram_auth.sh
            test_telegram_voice.sh
            test_telegram_format.sh
            # Scope 07: Daily digest
            test_digest.sh
            test_digest_quiet.sh
            test_digest_telegram.sh
            # Scope 08: Web UI
            test_web_ui.sh
            test_web_detail.sh
            test_web_settings.sh
            # Phase 2: Passive Ingestion (connectors + topics + settings)
            test_connector_framework.sh
            test_imap_sync.sh
            test_caldav_sync.sh
            test_youtube_sync.sh
            test_bookmark_import.sh
            test_topic_lifecycle.sh
            test_settings_connectors.sh
            # Phase 4: Expansion (maps + browser history)
            test_maps_import.sh
            test_browser_sync.sh
          )

          echo "Booting shared shell E2E test stack for ${#e2e_shared_scripts[@]} scripts..."
          e2e_down_test_stack "before shared shell E2E block"
          set +e
          e2e_run_child timeout 360 "$SCRIPT_DIR/smackerel.sh" --env test up
          e2e_shared_stack_status=$?
          set -e

          if [[ "$e2e_shared_stack_status" -ne 0 ]]; then
            e2e_record_shell_result "shared-stack-start" "$e2e_shared_stack_status"
          else
            for e2e_script in "${e2e_shared_scripts[@]}"; do
              echo "Running shared-stack shell E2E: $e2e_script"
              e2e_run_shell_test "$e2e_script" timeout 300 env E2E_STACK_MANAGED=1 bash "$SCRIPT_DIR/tests/e2e/$e2e_script"
            done
          fi

          echo "Tearing down shared shell E2E test stack..."
          e2e_shared_teardown_status=0
          e2e_down_test_stack "after shared shell E2E block" || e2e_shared_teardown_status=$?
          if [[ "$e2e_shared_teardown_status" -ne 0 ]]; then
            e2e_record_shell_result "shared-stack-teardown" "$e2e_shared_teardown_status"
          fi

          e2e_print_shell_summary
        fi
        # Go-based E2E tests (domain extraction, knowledge, capture-process-search)
        smackerel_generate_config test >/dev/null
        env_file="$(smackerel_require_env_file test)"
        core_host_port="$(smackerel_env_value "$env_file" "CORE_HOST_PORT")"
        pg_host_port="$(smackerel_env_value "$env_file" "POSTGRES_HOST_PORT")"
        nats_host_port="$(smackerel_env_value "$env_file" "NATS_CLIENT_HOST_PORT")"
        auth_token="$(smackerel_env_value "$env_file" "SMACKEREL_AUTH_TOKEN")"
        pg_user="$(smackerel_env_value "$env_file" "POSTGRES_USER")"
        pg_pass="$(smackerel_env_value "$env_file" "POSTGRES_PASSWORD")"
        pg_db="$(smackerel_env_value "$env_file" "POSTGRES_DB")"
        go_e2e_args=()
        if [[ -n "$GO_E2E_RUN_SELECTOR" ]]; then
          go_e2e_args+=(--run "$GO_E2E_RUN_SELECTOR")
        fi

        # Bring up a fresh stack for the Go E2E block; the e2e trap owns
        # final teardown regardless of Go test outcome.
        set +e
        e2e_run_child timeout 300 env KEEP_STACK_UP=1 bash "$SCRIPT_DIR/tests/integration/test_runtime_health.sh"
        e2e_go_stack_status=$?
        set -e
        if [[ "$e2e_go_stack_status" -ne 0 ]]; then
          echo "FAIL: go-e2e-stack-start (exit=${e2e_go_stack_status})"
          if [[ "$e2e_overall_status" -eq 0 ]]; then
            e2e_overall_status="$e2e_go_stack_status"
          fi
        else
          set +e
          e2e_run_child docker run --rm \
            --network host \
            -v "$SCRIPT_DIR:/workspace" \
            -v smackerel-gomod-cache:/go/pkg/mod \
            -v smackerel-gobuild-cache:/root/.cache/go-build \
            -w /workspace \
            -e "CORE_EXTERNAL_URL=http://127.0.0.1:${core_host_port}" \
            -e "DATABASE_URL=postgres://${pg_user}:${pg_pass}@127.0.0.1:${pg_host_port}/${pg_db}?sslmode=disable" \
            -e "POSTGRES_URL=postgres://${pg_user}:${pg_pass}@127.0.0.1:${pg_host_port}/${pg_db}?sslmode=disable" \
            -e "NATS_URL=nats://${auth_token}@127.0.0.1:${nats_host_port}" \
            -e "SMACKEREL_AUTH_TOKEN=${auth_token}" \
            golang:1.24.3-bookworm bash /workspace/scripts/runtime/go-e2e.sh "${go_e2e_args[@]}"
          e2e_go_status=$?
          set -e
          if [[ "$e2e_go_status" -eq 0 ]]; then
            echo "PASS: go-e2e"
          else
            echo "FAIL: go-e2e (exit=${e2e_go_status})"
            if [[ "$e2e_overall_status" -eq 0 ]]; then
              e2e_overall_status="$e2e_go_status"
            fi
          fi
        fi

        if [[ "$e2e_overall_status" -ne 0 ]]; then
          exit "$e2e_overall_status"
        fi
        ;;
      stress)
        timeout 300 bash "$SCRIPT_DIR/tests/stress/test_health_stress.sh"
        timeout 600 bash "$SCRIPT_DIR/tests/stress/test_search_stress.sh"
        ;;
      *)
        usage
        exit 1
        ;;
    esac
    ;;
  up)
    require_docker
    smackerel_generate_config "$TARGET_ENV" >/dev/null
    env_file="$(smackerel_require_env_file "$TARGET_ENV")"
    if [[ "$TARGET_ENV" == "test" ]]; then
      smackerel_prepare_test_stack_for_up "$env_file"
    fi
    compose_wait_timeout_s="$(smackerel_env_value "$env_file" "COMPOSE_WAIT_TIMEOUT_S")"
    if [[ -z "$compose_wait_timeout_s" ]]; then
      echo "ERROR: COMPOSE_WAIT_TIMEOUT_S missing from generated config" >&2
      exit 1
    fi
    smackerel_compose "$TARGET_ENV" up -d --wait --wait-timeout "$compose_wait_timeout_s"
    ;;
  down)
    require_docker
    smackerel_generate_config "$TARGET_ENV" >/dev/null
    if [[ "$DOWN_VOLUMES" == true ]]; then
      smackerel_compose "$TARGET_ENV" down --timeout 30 -v --remove-orphans
    else
      smackerel_compose "$TARGET_ENV" down --timeout 30 --remove-orphans
    fi
    ;;
  status)
    require_docker
    smackerel_generate_config "$TARGET_ENV" >/dev/null
    env_file="$(smackerel_require_env_file "$TARGET_ENV")"
    core_url="$(smackerel_env_value "$env_file" "CORE_EXTERNAL_URL")"
    smackerel_compose "$TARGET_ENV" ps
    curl --max-time 5 -fsS "$core_url/api/health"
    ;;
  logs)
    require_docker
    smackerel_generate_config "$TARGET_ENV" >/dev/null
    if [[ $# -gt 0 ]]; then
      smackerel_compose "$TARGET_ENV" logs "$1"
    else
      smackerel_compose "$TARGET_ENV" logs
    fi
    ;;
  clean)
    SUBCOMMAND="${1:-}"
    require_docker
    smackerel_generate_config "$TARGET_ENV" >/dev/null
    case "$SUBCOMMAND" in
      status)
        smackerel_compose "$TARGET_ENV" ps -a
        ;;
      measure)
        docker system df
        ;;
      smart)
        smackerel_compose "$TARGET_ENV" down --timeout 30 --remove-orphans
        ;;
      full)
        smackerel_compose "$TARGET_ENV" down --timeout 30 -v --remove-orphans
        ;;
      *)
        usage
        exit 1
        ;;
    esac
    ;;
  help)
    usage
    ;;
  *)
    usage
    exit 1
    ;;
esac