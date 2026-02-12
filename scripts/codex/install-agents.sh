#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
Usage: scripts/codex/install-agents.sh [--target DIR] [--dry-run]

Options:
  --target DIR  Destination agents directory (default: ~/.codex/agents)
  --dry-run     Show files without copying
USAGE
}

TARGET_DIR="${CODEX_AGENTS_DIR:-$HOME/.codex/agents}"
DRY_RUN=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      TARGET_DIR="$2"
      shift
      ;;
    --dry-run)
      DRY_RUN=true
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
SOURCE_DIR="$PROJECT_ROOT/.codex/agents"

if [[ ! -d "$SOURCE_DIR" ]]; then
  echo "Missing source directory: $SOURCE_DIR" >&2
  exit 1
fi

files=()
while IFS= read -r src; do
  files+=("$src")
done < <(find "$SOURCE_DIR" -maxdepth 1 -type f -name 'c4-*.md' | sort)

if [[ ${#files[@]} -eq 0 ]]; then
  echo "No agent files found in $SOURCE_DIR" >&2
  exit 1
fi

if [[ "$DRY_RUN" == true ]]; then
  echo "Dry run: would copy ${#files[@]} file(s) to $TARGET_DIR"
  printf '  %s\n' "${files[@]}"
  exit 0
fi

mkdir -p "$TARGET_DIR"
for src in "${files[@]}"; do
  install -m 0644 "$src" "$TARGET_DIR/"
  echo "Installed $(basename "$src")"
done

echo "Done. Installed ${#files[@]} C4 Codex agent file(s) to $TARGET_DIR"
