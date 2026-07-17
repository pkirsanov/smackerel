#!/usr/bin/env bash
set -euo pipefail

# adversarial-resolve.sh
#
# Adversarial-verification posture resolver (IMP-002 SCOPE-0 / control plane, C8).
#
# Resolves the EFFECTIVE adversarial posture (mode / samples / teeth) for a run
# from one layered precedence chain (highest wins), mirroring the shipped
# observability-endpoint-resolve.sh and the dual-trust resolver:
#
#   1. Per-run directive   (--mode/--samples/--teeth, or --directive "<string>")
#   2. Environment         (BUBBLES_ADVERSARIAL / _SAMPLES / _TEETH)
#   3. Project config      (.github/bubbles-project.yaml `adversarial:` block)
#   4. Framework default   (mode=off, samples=1, teeth=warn)
#
# The capability is OFF BY DEFAULT (mode=off): a zero-config repo resolves to
# `off` and adversarial verification does nothing — zero behavior change on
# upgrade. A team opts in seamlessly: session/CI via BUBBLES_ADVERSARIAL=auto,
# repo-wide via the `adversarial:` config block, or for a single agent/workflow
# run via the directive layer.
#
# Vocabulary:
#   mode   = off  → never run; auto → run only on high-risk scopes (riskClass
#                   gated); on → run for this scope regardless of risk.
#   samples = N   → number of correlated evaluations requested in the active
#                   runtime. One sample is the normal default. Positive integer.
#   teeth  = warn → a red-team counterexample is a recorded finding (default,
#                   grandfathered like G101); blocking → it blocks certification.
#
# `passes` is a deprecated compatibility alias at every input layer. Canonical
# `samples` wins when both are present at the same layer. Alias use is reported
# on stderr and in the stable `deprecation` output field.
#
# Parser dependency: `yq` (mikefarah v4) for the config layer ONLY. A missing yq
# WARN-and-skips the config layer (stderr note, resolution continues) — a missing
# developer tool must not hard-fail a resolution helper. Directive + env + the
# off-by-default still resolve without yq.
#
# There is NO bypass flag. `--skip` / `--force` / `--ignore` do not exist.
#
# Output (stdout) — eval-friendly KEY=VALUE lines:
#   mode=<off|auto|on>
#   samples=<N>
#   sampleSemantics=same-runtime-correlated
#   teeth=<warn|blocking>
#   source=<directive|env|config|default>   (which layer set `mode`)
#   samplesSource=<directive|env|config|default>
#   deprecation=<none|passes-alias>
#
# Exit codes:
#   0  resolved (incl. mode=off and yq-missing config WARN-and-skip)
#   1  validation failure (invalid mode/samples/teeth)
#   2  usage error (unknown flag / missing required option value)
#
# Reference: improvements/IMP-020-agentic-evaluation-and-trust-hardening.md (S2)

DEFAULT_MODE="off"
DEFAULT_SAMPLES="1"
DEFAULT_TEETH="warn"
SAMPLE_SEMANTICS="same-runtime-correlated"

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"

REPO_ROOT_ARG=""
DIR_MODE=""
DIR_SAMPLES=""
DIR_PASSES=""
DIR_TEETH=""
DIRECTIVE_STR=""
MODE_FLAG_SEEN=0
SAMPLES_FLAG_SEEN=0
PASSES_FLAG_SEEN=0
TEETH_FLAG_SEEN=0
DIRECTIVE_FLAG_SEEN=0

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/adversarial-resolve.sh [--mode <m>] [--samples <n>] [--passes <n>] [--teeth <t>] [--directive "<str>"] [--repo-root <dir>]

Resolve the effective adversarial posture (mode/samples/teeth) from the precedence
chain: per-run directive -> BUBBLES_ADVERSARIAL* env -> bubbles-project.yaml
`adversarial:` block -> framework default (off).

Options:
  --mode <off|auto|on>      Per-run mode (directive layer, highest precedence).
  --samples <N>             Per-run correlated sample count (integer 1..5).
  --passes <N>              Deprecated compatibility alias for --samples.
  --teeth <warn|blocking>   Per-run teeth.
  --directive "<str>"       Free-form per-run string; mode:/samples:/teeth: tokens
                            are extracted (what an orchestrator forwards from
                            $ADDITIONAL_CONTEXT, e.g. "adversarial: on samples: 3").
                            The deprecated passes: token remains accepted.
                            Explicit --mode/--samples/--passes/--teeth override the same
                            token inside --directive.
  --repo-root <dir>         Repo root to scan for bubbles-project.yaml
                            (default: the repo this script lives in).
  -h, --help                Print this usage and exit 0.

Exit codes: 0 resolved (incl. off / yq-missing) | 1 invalid value | 2 usage.
There is NO --skip/--force/--ignore bypass. OFF BY DEFAULT.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --mode)
      if [[ "$MODE_FLAG_SEEN" -eq 1 ]]; then
        echo "adversarial-resolve: duplicate --mode flag is ambiguous" >&2
        exit 2
      fi
      MODE_FLAG_SEEN=1
      shift
      [[ $# -gt 0 ]] || { echo "adversarial-resolve: --mode requires a value" >&2; exit 2; }
      DIR_MODE="$1"
      shift
      ;;
    --passes)
      if [[ "$PASSES_FLAG_SEEN" -eq 1 ]]; then
        echo "adversarial-resolve: duplicate --passes flag is ambiguous" >&2
        exit 2
      fi
      PASSES_FLAG_SEEN=1
      shift
      [[ $# -gt 0 ]] || { echo "adversarial-resolve: --passes requires a value" >&2; exit 2; }
      DIR_PASSES="$1"
      shift
      ;;
    --samples)
      if [[ "$SAMPLES_FLAG_SEEN" -eq 1 ]]; then
        echo "adversarial-resolve: duplicate --samples flag is ambiguous" >&2
        exit 2
      fi
      SAMPLES_FLAG_SEEN=1
      shift
      [[ $# -gt 0 ]] || { echo "adversarial-resolve: --samples requires a value" >&2; exit 2; }
      DIR_SAMPLES="$1"
      shift
      ;;
    --teeth)
      if [[ "$TEETH_FLAG_SEEN" -eq 1 ]]; then
        echo "adversarial-resolve: duplicate --teeth flag is ambiguous" >&2
        exit 2
      fi
      TEETH_FLAG_SEEN=1
      shift
      [[ $# -gt 0 ]] || { echo "adversarial-resolve: --teeth requires a value" >&2; exit 2; }
      DIR_TEETH="$1"
      shift
      ;;
    --directive)
      if [[ "$DIRECTIVE_FLAG_SEEN" -eq 1 ]]; then
        echo "adversarial-resolve: duplicate --directive flag is ambiguous" >&2
        exit 2
      fi
      DIRECTIVE_FLAG_SEEN=1
      shift
      [[ $# -gt 0 ]] || { echo "adversarial-resolve: --directive requires a value" >&2; exit 2; }
      DIRECTIVE_STR="$1"
      shift
      ;;
    --repo-root)
      shift
      [[ $# -gt 0 ]] || { echo "adversarial-resolve: --repo-root requires a value" >&2; exit 2; }
      REPO_ROOT_ARG="$1"
      shift
      ;;
    *)
      echo "adversarial-resolve: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

# Parse standalone directive tokens once into a private record. Keys embedded
# in ASCII identifiers (including underscore and hyphen) are ignored.
DIRECTIVE_RECORD=""
DIR_MODE_TOKEN_COUNT=0
DIR_MODE_TOKEN=""
DIR_SAMPLES_TOKEN_COUNT=0
DIR_SAMPLES_TOKEN=""
DIR_PASSES_TOKEN_COUNT=0
DIR_PASSES_TOKEN=""
DIR_TEETH_TOKEN_COUNT=0
DIR_TEETH_TOKEN=""

remove_directive_record() {
  [[ -n "$DIRECTIVE_RECORD" ]] || return 0
  if ! rm -f "$DIRECTIVE_RECORD" 2>/dev/null; then
    echo "adversarial-resolve: directive parser record cleanup failed" >&2
    return 1
  fi
  DIRECTIVE_RECORD=""
}

cleanup_directive_record() {
  local cleanup_status=$?
  trap - EXIT
  if ! remove_directive_record; then
    if [[ "$cleanup_status" -eq 0 ]]; then
      cleanup_status=2
    fi
  fi
  exit "$cleanup_status"
}

directive_parser_failed() {
  echo "adversarial-resolve: directive parser failed" >&2
  exit 2
}

config_parser_failed() {
  echo "adversarial-resolve: config parser failed" >&2
  exit 2
}

parse_directive() {
  local record_content record_key record_line record_read_status record_value

  if ! DIRECTIVE_RECORD="$(mktemp "${TMPDIR:-/tmp}/bubbles-adversarial-resolve.XXXXXXXX" 2>/dev/null)"; then
    directive_parser_failed
  fi
  trap cleanup_directive_record EXIT
  trap 'exit 129' HUP
  trap 'exit 130' INT
  trap 'exit 143' TERM

  if ! printf '%s\n' "$DIRECTIVE_STR" | awk '
    function starts_known_key(start, key_index, key, cursor) {
      for (key_index = 1; key_index <= known_key_count; key_index++) {
        key = known_keys[key_index]
        if (substr(text, start, length(key)) != key) continue
        cursor = start + length(key)
        if (substr(text, cursor, 1) ~ /[A-Za-z0-9_-]/) continue
        while (substr(text, cursor, 1) ~ /[[:space:]]/) cursor++
        if (substr(text, cursor, 1) == ":" || substr(text, cursor, 1) == "=") return 1
      }
      return 0
    }
    BEGIN {
      known_key_count = split("adversarial|mode|samples|passes|teeth", known_keys, /[|]/)
    }
    {
      text = tolower($0)
      text_length = length(text)
      for (position = 1; position <= text_length; position++) {
        previous = position > 1 ? substr(text, position - 1, 1) : ""
        if (previous ~ /[A-Za-z0-9_-]/) continue

        for (key_index = 1; key_index <= known_key_count; key_index++) {
          logical_index = key_index <= 2 ? 1 : key_index - 1
          if (position <= skip_until[logical_index]) continue

          key = known_keys[key_index]
          if (substr(text, position, length(key)) != key) continue

          cursor = position + length(key)
          if (substr(text, cursor, 1) ~ /[A-Za-z0-9_-]/) continue
          while (substr(text, cursor, 1) ~ /[[:space:]]/) cursor++
          separator = substr(text, cursor, 1)
          if (separator != ":" && separator != "=") continue

          cursor++
          while (substr(text, cursor, 1) ~ /[[:space:]]/) cursor++
          output_key = logical_index == 1 ? "mode" : key
          if (cursor > text_length || starts_known_key(cursor)) {
            printf "%s\t\n", output_key
            skip_until[logical_index] = cursor - 1
            break
          }
          value_start = cursor
          while (substr(text, cursor, 1) ~ /[^[:space:]]/) cursor++

          printf "%s\t%s\n", output_key, substr(text, value_start, cursor - value_start)
          skip_until[logical_index] = cursor - 1
          break
        }
      }
    }
  ' 2>/dev/null > "$DIRECTIVE_RECORD"; then
    directive_parser_failed
  fi

  if [[ ! -f "$DIRECTIVE_RECORD" || -L "$DIRECTIVE_RECORD" || ! -r "$DIRECTIVE_RECORD" ]]; then
    directive_parser_failed
  fi
  if ! record_content="$(
    cat "$DIRECTIVE_RECORD" 2>/dev/null
    record_read_status=$?
    if [[ "$record_read_status" -ne 0 ]]; then
      exit "$record_read_status"
    fi
    printf '\034'
  )"; then
    directive_parser_failed
  fi
  if [[ ! -f "$DIRECTIVE_RECORD" || -L "$DIRECTIVE_RECORD" || ! -r "$DIRECTIVE_RECORD" ]]; then
    directive_parser_failed
  fi
  if [[ "$record_content" != *$'\034' ]]; then
    directive_parser_failed
  fi
  record_content="${record_content%$'\034'}"
  if [[ -n "$record_content" && "$record_content" != *$'\n' ]]; then
    directive_parser_failed
  fi

  while [[ -n "$record_content" ]]; do
    record_line="${record_content%%$'\n'*}"
    record_content="${record_content#*$'\n'}"
    if [[ "$record_line" != *$'\t'* ]]; then
      directive_parser_failed
    fi
    record_key="${record_line%%$'\t'*}"
    record_value="${record_line#*$'\t'}"
    if [[ "$record_value" == *$'\t'* ]] \
      || [[ "$record_key" =~ [[:cntrl:]] ]] \
      || [[ "$record_value" =~ [[:cntrl:]] ]]; then
      directive_parser_failed
    fi
    case "$record_key" in
      mode)
        DIR_MODE_TOKEN_COUNT=$((DIR_MODE_TOKEN_COUNT + 1))
        [[ "$DIR_MODE_TOKEN_COUNT" -ne 1 ]] || DIR_MODE_TOKEN="$record_value"
        ;;
      samples)
        DIR_SAMPLES_TOKEN_COUNT=$((DIR_SAMPLES_TOKEN_COUNT + 1))
        [[ "$DIR_SAMPLES_TOKEN_COUNT" -ne 1 ]] || DIR_SAMPLES_TOKEN="$record_value"
        ;;
      passes)
        DIR_PASSES_TOKEN_COUNT=$((DIR_PASSES_TOKEN_COUNT + 1))
        [[ "$DIR_PASSES_TOKEN_COUNT" -ne 1 ]] || DIR_PASSES_TOKEN="$record_value"
        ;;
      teeth)
        DIR_TEETH_TOKEN_COUNT=$((DIR_TEETH_TOKEN_COUNT + 1))
        [[ "$DIR_TEETH_TOKEN_COUNT" -ne 1 ]] || DIR_TEETH_TOKEN="$record_value"
        ;;
      *) directive_parser_failed ;;
    esac
  done
}

YQ_QUERY_LINE=""
capture_yq_line() {
  local framed_output query_status

  if ! framed_output="$(
    yq "$1" "$config_file" 2>/dev/null
    query_status=$?
    if [[ "$query_status" -ne 0 ]]; then
      exit "$query_status"
    fi
    printf '\034'
  )"; then
    config_parser_failed
  fi
  if [[ "$framed_output" != *$'\034' ]]; then
    config_parser_failed
  fi
  framed_output="${framed_output%$'\034'}"
  if [[ "$framed_output" != *$'\n' ]]; then
    config_parser_failed
  fi
  framed_output="${framed_output%$'\n'}"
  if [[ "$framed_output" == *$'\n'* ]] \
    || [[ "$framed_output" =~ [[:cntrl:]] ]]; then
    config_parser_failed
  fi
  YQ_QUERY_LINE="$framed_output"
}

CONFIG_FIELD_VALUE=""
CONFIG_FIELD_PRESENT=0
validate_config_field() {
  local field="$1"
  local value="$2"
  local tag="$3"

  CONFIG_FIELD_VALUE="$value"
  CONFIG_FIELD_PRESENT=0
  case "$field" in
    mode|teeth)
      case "$tag" in
        '!!null'|'!!str') ;;
        *) config_parser_failed ;;
      esac
      ;;
    samples|passes)
      case "$tag" in
        '!!null'|'!!str'|'!!int') ;;
        *) config_parser_failed ;;
      esac
      ;;
    *) config_parser_failed ;;
  esac

  case "$tag" in
    '!!null')
      if [[ "$value" != "null" ]]; then
        config_parser_failed
      fi
      CONFIG_FIELD_VALUE=""
      ;;
    '!!str')
      CONFIG_FIELD_PRESENT=1
      ;;
    '!!int')
      if ! [[ "$value" =~ ^-?[0-9]+$ ]]; then
        config_parser_failed
      fi
      CONFIG_FIELD_PRESENT=1
      ;;
    *) config_parser_failed ;;
  esac
}

validate_directive_duplicates() {
  [[ -n "$DIRECTIVE_STR" ]] || return 0
  if [[ "$DIR_MODE_TOKEN_COUNT" -gt 1 ]]; then
    echo "adversarial-resolve: duplicate mode directive token is ambiguous" >&2
    exit 2
  fi
  if [[ "$DIR_SAMPLES_TOKEN_COUNT" -gt 1 ]]; then
    echo "adversarial-resolve: duplicate samples directive token is ambiguous" >&2
    exit 2
  fi
  if [[ "$DIR_PASSES_TOKEN_COUNT" -gt 1 ]]; then
    echo "adversarial-resolve: duplicate passes directive token is ambiguous" >&2
    exit 2
  fi
  if [[ "$DIR_TEETH_TOKEN_COUNT" -gt 1 ]]; then
    echo "adversarial-resolve: duplicate teeth directive token is ambiguous" >&2
    exit 2
  fi
}

parse_directive

# Resolve repo root.
REPO_ROOT="${REPO_ROOT_ARG:-}"
if [[ -z "$REPO_ROOT" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd)"
fi

# --- Config layer: adversarial.{mode,samples,teeth} from bubbles-project.yaml ---
CFG_MODE=""
CFG_MODE_PRESENT=0
CFG_SAMPLES=""
CFG_SAMPLES_PRESENT=0
CFG_PASSES=""
CFG_PASSES_PRESENT=0
CFG_TEETH=""
CFG_TEETH_PRESENT=0
config_file=""
for c in "$REPO_ROOT/.github/bubbles-project.yaml" "$REPO_ROOT/bubbles-project.yaml"; do
  if [[ -f "$c" ]]; then
    config_file="$c"
    break
  fi
done
if [[ -n "$config_file" ]]; then
  if command -v yq >/dev/null 2>&1; then
    capture_yq_line '.adversarial.mode'
    CFG_MODE_RAW="$YQ_QUERY_LINE"
    capture_yq_line '.adversarial.mode | tag'
    CFG_MODE_TAG="$YQ_QUERY_LINE"
    capture_yq_line '.adversarial.samples'
    CFG_SAMPLES_RAW="$YQ_QUERY_LINE"
    capture_yq_line '.adversarial.samples | tag'
    CFG_SAMPLES_TAG="$YQ_QUERY_LINE"
    capture_yq_line '.adversarial.passes'
    CFG_PASSES_RAW="$YQ_QUERY_LINE"
    capture_yq_line '.adversarial.passes | tag'
    CFG_PASSES_TAG="$YQ_QUERY_LINE"
    capture_yq_line '.adversarial.teeth'
    CFG_TEETH_RAW="$YQ_QUERY_LINE"
    capture_yq_line '.adversarial.teeth | tag'
    CFG_TEETH_TAG="$YQ_QUERY_LINE"

    validate_config_field mode "$CFG_MODE_RAW" "$CFG_MODE_TAG"
    CFG_MODE_CHECKED="$CONFIG_FIELD_VALUE"
    CFG_MODE_PRESENT_CHECKED="$CONFIG_FIELD_PRESENT"
    validate_config_field samples "$CFG_SAMPLES_RAW" "$CFG_SAMPLES_TAG"
    CFG_SAMPLES_CHECKED="$CONFIG_FIELD_VALUE"
    CFG_SAMPLES_PRESENT_CHECKED="$CONFIG_FIELD_PRESENT"
    validate_config_field passes "$CFG_PASSES_RAW" "$CFG_PASSES_TAG"
    CFG_PASSES_CHECKED="$CONFIG_FIELD_VALUE"
    CFG_PASSES_PRESENT_CHECKED="$CONFIG_FIELD_PRESENT"
    validate_config_field teeth "$CFG_TEETH_RAW" "$CFG_TEETH_TAG"
    CFG_TEETH_CHECKED="$CONFIG_FIELD_VALUE"
    CFG_TEETH_PRESENT_CHECKED="$CONFIG_FIELD_PRESENT"

    CFG_MODE="$CFG_MODE_CHECKED"
    CFG_MODE_PRESENT="$CFG_MODE_PRESENT_CHECKED"
    CFG_SAMPLES="$CFG_SAMPLES_CHECKED"
    CFG_SAMPLES_PRESENT="$CFG_SAMPLES_PRESENT_CHECKED"
    CFG_PASSES="$CFG_PASSES_CHECKED"
    CFG_PASSES_PRESENT="$CFG_PASSES_PRESENT_CHECKED"
    CFG_TEETH="$CFG_TEETH_CHECKED"
    CFG_TEETH_PRESENT="$CFG_TEETH_PRESENT_CHECKED"
  else
    echo "adversarial-resolve: yq not found — skipping config layer (directive/env/default still apply)" >&2
  fi
fi

# --- Directive layer (explicit flags override --directive tokens) ---
validate_directive_duplicates
DIR_MODE_PRESENT="$MODE_FLAG_SEEN"
DIR_SAMPLES_PRESENT="$SAMPLES_FLAG_SEEN"
DIR_PASSES_PRESENT="$PASSES_FLAG_SEEN"
DIR_TEETH_PRESENT="$TEETH_FLAG_SEEN"
if [[ "$DIR_MODE_PRESENT" -eq 0 ]]; then
  if [[ "$DIR_MODE_TOKEN_COUNT" -gt 0 ]]; then
    DIR_MODE_PRESENT=1
    DIR_MODE="$DIR_MODE_TOKEN"
  fi
fi
if [[ "$DIR_SAMPLES_PRESENT" -eq 0 ]]; then
  if [[ "$DIR_SAMPLES_TOKEN_COUNT" -gt 0 ]]; then
    DIR_SAMPLES_PRESENT=1
    DIR_SAMPLES="$DIR_SAMPLES_TOKEN"
  fi
fi
if [[ "$DIR_PASSES_PRESENT" -eq 0 ]]; then
  if [[ "$DIR_PASSES_TOKEN_COUNT" -gt 0 ]]; then
    DIR_PASSES_PRESENT=1
    DIR_PASSES="$DIR_PASSES_TOKEN"
  fi
fi
if [[ "$DIR_TEETH_PRESENT" -eq 0 ]]; then
  if [[ "$DIR_TEETH_TOKEN_COUNT" -gt 0 ]]; then
    DIR_TEETH_PRESENT=1
    DIR_TEETH="$DIR_TEETH_TOKEN"
  fi
fi

# --- Env layer ---
ENV_MODE=""
ENV_MODE_PRESENT=0
if [[ -n "${BUBBLES_ADVERSARIAL+x}" ]]; then
  ENV_MODE="$BUBBLES_ADVERSARIAL"
  ENV_MODE_PRESENT=1
fi
ENV_SAMPLES=""
ENV_SAMPLES_PRESENT=0
if [[ -n "${BUBBLES_ADVERSARIAL_SAMPLES+x}" ]]; then
  ENV_SAMPLES="$BUBBLES_ADVERSARIAL_SAMPLES"
  ENV_SAMPLES_PRESENT=1
fi
ENV_PASSES=""
ENV_PASSES_PRESENT=0
if [[ -n "${BUBBLES_ADVERSARIAL_PASSES+x}" ]]; then
  ENV_PASSES="$BUBBLES_ADVERSARIAL_PASSES"
  ENV_PASSES_PRESENT=1
fi
ENV_TEETH=""
ENV_TEETH_PRESENT=0
if [[ -n "${BUBBLES_ADVERSARIAL_TEETH+x}" ]]; then
  ENV_TEETH="$BUBBLES_ADVERSARIAL_TEETH"
  ENV_TEETH_PRESENT=1
fi

validate_count() {
  local label="$1" value="$2"
  if ! [[ "$value" =~ ^[1-5]$ ]]; then
    echo "adversarial-resolve: invalid ${label} '${value}' (expected integer 1..5)" >&2
    exit 1
  fi
}

# --- Precedence resolution: directive > env > config > default ---
MODE="$DEFAULT_MODE"
MODE_SRC="default"
if [[ "$DIR_MODE_PRESENT" -eq 1 ]]; then
  MODE="$DIR_MODE"
  MODE_SRC="directive"
elif [[ "$ENV_MODE_PRESENT" -eq 1 ]]; then
  MODE="$ENV_MODE"
  MODE_SRC="env"
elif [[ "$CFG_MODE_PRESENT" -eq 1 ]]; then
  MODE="$CFG_MODE"
  MODE_SRC="config"
fi

TEETH="$DEFAULT_TEETH"
if [[ "$DIR_TEETH_PRESENT" -eq 1 ]]; then
  TEETH="$DIR_TEETH"
elif [[ "$ENV_TEETH_PRESENT" -eq 1 ]]; then
  TEETH="$ENV_TEETH"
elif [[ "$CFG_TEETH_PRESENT" -eq 1 ]]; then
  TEETH="$CFG_TEETH"
fi

SAMPLES="$DEFAULT_SAMPLES"
SAMPLES_SRC="default"
SAMPLES_KIND="samples"
if [[ "$DIR_SAMPLES_PRESENT" -eq 1 ]]; then
  SAMPLES="$DIR_SAMPLES"
  SAMPLES_SRC="directive"
elif [[ "$DIR_PASSES_PRESENT" -eq 1 ]]; then
  SAMPLES="$DIR_PASSES"
  SAMPLES_SRC="directive"
  SAMPLES_KIND="passes"
elif [[ "$ENV_SAMPLES_PRESENT" -eq 1 ]]; then
  SAMPLES="$ENV_SAMPLES"
  SAMPLES_SRC="env"
elif [[ "$ENV_PASSES_PRESENT" -eq 1 ]]; then
  SAMPLES="$ENV_PASSES"
  SAMPLES_SRC="env"
  SAMPLES_KIND="passes"
elif [[ "$CFG_SAMPLES_PRESENT" -eq 1 ]]; then
  SAMPLES="$CFG_SAMPLES"
  SAMPLES_SRC="config"
elif [[ "$CFG_PASSES_PRESENT" -eq 1 ]]; then
  SAMPLES="$CFG_PASSES"
  SAMPLES_SRC="config"
  SAMPLES_KIND="passes"
fi

case "$SAMPLES_SRC" in
  directive)
    if [[ "$SAMPLES_KIND" == "samples" ]]; then SAMPLES_LABEL="samples"; else SAMPLES_LABEL="passes alias"; fi
    ;;
  env)
    if [[ "$SAMPLES_KIND" == "samples" ]]; then SAMPLES_LABEL="BUBBLES_ADVERSARIAL_SAMPLES"; else SAMPLES_LABEL="BUBBLES_ADVERSARIAL_PASSES alias"; fi
    ;;
  config)
    if [[ "$SAMPLES_KIND" == "samples" ]]; then SAMPLES_LABEL="config adversarial.samples"; else SAMPLES_LABEL="config adversarial.passes alias"; fi
    ;;
  default) SAMPLES_LABEL="samples" ;;
esac

MODE="$(printf '%s' "$MODE" | tr '[:upper:]' '[:lower:]')"
TEETH="$(printf '%s' "$TEETH" | tr '[:upper:]' '[:lower:]')"

# --- Validate ---
validate_count "$SAMPLES_LABEL" "$SAMPLES"
case "$MODE" in
  off|auto|on) ;;
  *) echo "adversarial-resolve: invalid mode '$MODE' (expected off|auto|on)" >&2; exit 1 ;;
esac
case "$TEETH" in
  warn|blocking) ;;
  *) echo "adversarial-resolve: invalid teeth '$TEETH' (expected warn|blocking)" >&2; exit 1 ;;
esac
DEPRECATION="none"
deprecated_layers=""
append_deprecated_layer() {
  local layer="$1"
  if [[ -n "$deprecated_layers" ]]; then
    deprecated_layers="${deprecated_layers},${layer}"
  else
    deprecated_layers="$layer"
  fi
}
[[ "$DIR_PASSES_PRESENT" -eq 0 ]] || append_deprecated_layer "directive"
[[ "$ENV_PASSES_PRESENT" -eq 0 ]] || append_deprecated_layer "env"
[[ "$CFG_PASSES_PRESENT" -eq 0 ]] || append_deprecated_layer "config"
if [[ -n "$deprecated_layers" ]]; then
  DEPRECATION="passes-alias"
  echo "adversarial-resolve: DEPRECATED: passes alias used at layer(s): ${deprecated_layers}; use samples instead" >&2
fi

if ! remove_directive_record; then
  trap - EXIT
  exit 2
fi
trap - EXIT

printf 'mode=%s\n' "$MODE"
printf 'samples=%s\n' "$SAMPLES"
printf 'sampleSemantics=%s\n' "$SAMPLE_SEMANTICS"
printf 'teeth=%s\n' "$TEETH"
printf 'source=%s\n' "$MODE_SRC"
printf 'samplesSource=%s\n' "$SAMPLES_SRC"
printf 'deprecation=%s\n' "$DEPRECATION"
