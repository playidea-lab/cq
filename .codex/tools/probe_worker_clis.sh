#!/usr/bin/env bash
set -euo pipefail

check_cli() {
  local name="$1"
  shift
  local cmd=("$@")

  if ! command -v "${cmd[0]}" >/dev/null 2>&1; then
    printf '%-12s : not-installed\n' "$name"
    return
  fi

  local help
  if ! help="$("${cmd[@]}" 2>&1 || true)"; then
    help=""
  fi

  local score=0
  [[ "$help" =~ exec|run ]] && score=$((score + 1))
  [[ "$help" =~ --json|-j ]] && score=$((score + 1))
  [[ "$help" =~ --prompt|-p|--input ]] && score=$((score + 1))

  local rating="low"
  if (( score >= 3 )); then
    rating="high"
  elif (( score == 2 )); then
    rating="medium"
  fi

  printf '%-12s : installed, capability=%s (score=%d)\n' "$name" "$rating" "$score"
}

check_cli "codex" codex --help
check_cli "claude" claude --help
check_cli "gemini" gemini --help
check_cli "opencode" opencode --help
