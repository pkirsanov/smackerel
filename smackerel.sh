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
  check                       Validate generated config and docker-compose wiring
  lint                        Run Go vet and Python ruff inside containers
  format [--check]            Format Go and Python files, or check formatting
  test unit [--go|--python]   Run unit tests
  test integration            Run live-stack integration validation
  test e2e                    Run E2E scaffold tests
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
  docker run --rm -v "$SCRIPT_DIR:/workspace" -w /workspace golang:1.24.3-bookworm bash "$script_path" "$@"
}

run_python_tooling() {
  local script_path="$1"
  shift || true
  docker run --rm -v "$SCRIPT_DIR:/workspace" -w /workspace python:3.12-slim bash "$script_path" "$@"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env)
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
      break
      ;;
  esac
done

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
    smackerel_compose "$TARGET_ENV" "${build_args[@]}"
    ;;
  check)
    require_docker
    smackerel_generate_config "$TARGET_ENV" >/dev/null
    smackerel_compose "$TARGET_ENV" config -q
    ;;
  lint)
    run_go_tooling /workspace/scripts/runtime/go-lint.sh
    run_python_tooling /workspace/scripts/runtime/python-lint.sh
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
        timeout 300 bash "$SCRIPT_DIR/tests/integration/test_runtime_health.sh"
        ;;
      e2e)
        timeout 300 bash "$SCRIPT_DIR/tests/e2e/test_compose_start.sh"
        timeout 300 bash "$SCRIPT_DIR/tests/e2e/test_persistence.sh"
        timeout 300 bash "$SCRIPT_DIR/tests/e2e/test_config_fail.sh"
        ;;
      stress)
        timeout 300 bash "$SCRIPT_DIR/tests/stress/test_health_stress.sh"
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
    smackerel_compose "$TARGET_ENV" up -d
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