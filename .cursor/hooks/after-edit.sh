#!/usr/bin/env bash
# C4: After file edit - lint/format by extension. Receives JSON with file_path on stdin.
set -e
input=$(cat)
path=$(echo "$input" | jq -r '.file_path // empty')
[[ -z "$path" ]] && exit 0
ext="${path##*.}"

case "$ext" in
  py)
    if command -v uv &>/dev/null; then
      uv run ruff check "$path" --fix 2>/dev/null || true
      uv run ruff format "$path" 2>/dev/null || true
    fi
    ;;
  ts|tsx)
    if [[ -f "package.json" ]]; then
      npx eslint --fix "$path" 2>/dev/null || true
      npx prettier --write "$path" 2>/dev/null || true
      npx tsc --noEmit 2>/dev/null || true
    fi
    if grep -q 'console\.log' "$path" 2>/dev/null; then
      echo "C4: console.log found in $path - consider removing before commit" >&2
    fi
    ;;
  go)
    if [[ -d "c4-core" ]]; then
      (cd c4-core && go vet ./... 2>&1 | head -20) || true
    fi
    ;;
  js)
    if [[ -f "package.json" ]]; then
      npx eslint --fix "$path" 2>/dev/null || true
      npx prettier --write "$path" 2>/dev/null || true
    fi
    if grep -q 'console\.log' "$path" 2>/dev/null; then
      echo "C4: console.log found in $path - consider removing before commit" >&2
    fi
    ;;
  *) ;;
esac
exit 0
