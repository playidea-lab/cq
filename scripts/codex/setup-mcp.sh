#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
Usage: scripts/codex/setup-mcp.sh [--write] [--force] [--print]

Options:
  --write   Append/update c4 MCP block in ~/.codex/config.toml
  --force   Replace existing [mcp_servers.c4] block
  --print   Print only the generated TOML block
USAGE
}

WRITE=false
FORCE=false
PRINT_ONLY=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --write)
      WRITE=true
      ;;
    --force)
      FORCE=true
      ;;
    --print)
      PRINT_ONLY=true
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
  shift
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
C4_BIN="$PROJECT_ROOT/c4-core/bin/c4"
CONFIG_FILE="${CODEX_CONFIG:-$HOME/.codex/config.toml}"

if [[ ! -x "$C4_BIN" ]]; then
  cat >&2 <<MSG
c4 binary not found: $C4_BIN
Build it first:
  (cd "$PROJECT_ROOT/c4-core" && go build -o bin/c4 ./cmd/c4)
MSG
  exit 1
fi

SNIPPET=$(cat <<TOML
[mcp_servers.c4]
command = "$C4_BIN"
args = ["mcp", "--dir", "$PROJECT_ROOT"]
env = { C4_PROJECT_ROOT = "$PROJECT_ROOT" }
TOML
)

if [[ "$PRINT_ONLY" == true ]]; then
  printf '%s\n' "$SNIPPET"
  exit 0
fi

existing=false
if [[ -f "$CONFIG_FILE" ]] && grep -Eq '^\[mcp_servers\.c4\]' "$CONFIG_FILE"; then
  existing=true
fi

if [[ "$WRITE" != true ]]; then
  if [[ "$existing" == true ]]; then
    echo "c4 MCP block already exists in $CONFIG_FILE"
  else
    echo "Generated block (not written):"
    printf '%s\n' "$SNIPPET"
    echo
    echo "Write with: scripts/codex/setup-mcp.sh --write"
  fi
  exit 0
fi

mkdir -p "$(dirname "$CONFIG_FILE")"
touch "$CONFIG_FILE"

if [[ "$existing" == true ]]; then
  if [[ "$FORCE" != true ]]; then
    echo "c4 MCP block already exists in $CONFIG_FILE"
    echo "Use --force to replace it."
    exit 1
  fi

  tmp_file="$(mktemp)"
  awk '
    BEGIN { skip = 0 }
    /^\[mcp_servers\.c4\]$/ { skip = 1; next }
    skip == 1 && /^\[/ { skip = 0 }
    skip == 0 { print }
  ' "$CONFIG_FILE" > "$tmp_file"

  # Ensure trailing newline before appending.
  if [[ -s "$tmp_file" ]]; then
    printf '\n' >> "$tmp_file"
  fi
  printf '%s\n' "$SNIPPET" >> "$tmp_file"
  mv "$tmp_file" "$CONFIG_FILE"
  echo "Replaced c4 MCP block in $CONFIG_FILE"
  exit 0
fi

if [[ -s "$CONFIG_FILE" ]]; then
  printf '\n' >> "$CONFIG_FILE"
fi
printf '%s\n' "$SNIPPET" >> "$CONFIG_FILE"
echo "Added c4 MCP block to $CONFIG_FILE"
