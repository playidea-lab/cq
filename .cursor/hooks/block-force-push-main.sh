#!/usr/bin/env bash
# C4: Block force push to main/master. Exit 2 to block.
set -e
input=$(cat)
cmd=$(echo "$input" | jq -r '.command // empty')
if [[ "$cmd" != *"git push"* ]]; then
  echo '{"permission":"allow"}'
  exit 0
fi
if [[ "$cmd" == *"--force"* ]] || [[ "$cmd" == *"-f"* ]]; then
  if [[ "$cmd" == *"main"* ]] || [[ "$cmd" == *"master"* ]]; then
    echo '{"permission":"deny","user_message":"C4: Force push to main/master is not allowed."}'
    exit 2
  fi
fi
echo '{"permission":"allow"}'
exit 0
