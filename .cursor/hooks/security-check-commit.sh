#!/usr/bin/env bash
# C4: Scan for hardcoded secrets before git commit. Exit 2 to block.
set -e
input=$(cat)
cmd=$(echo "$input" | jq -r '.command // empty')
if [[ "$cmd" != *"git commit"* ]]; then
  echo '{"permission":"allow"}'
  exit 0
fi
found=$(grep -rn --include='*.py' --include='*.ts' --include='*.js' \
  -E '(password|api_key|secret|token)\s*=\s*["'"'"'][^"'"'"']+["'"'"']' . 2>/dev/null \
  | grep -v -E '(test_|_test\.py|spec\.ts|\.env\.example|node_modules|/vendor/|\.d\.ts|\.venv/|site-packages/)' \
  | head -5 || true)
if [[ -n "$found" ]]; then
  echo '{"permission":"deny","user_message":"C4: Possible hardcoded secrets found. Fix or add to .env.example before commit."}'
  exit 2
fi
echo '{"permission":"allow"}'
exit 0
