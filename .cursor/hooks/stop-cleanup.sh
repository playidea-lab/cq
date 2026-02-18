#!/usr/bin/env bash
# C4: Session end - check for TODO: remove markers.
set -e
if grep -rn --include='*.py' --include='*.ts' --include='*.js' 'TODO: remove' . 2>/dev/null | head -5; then
  echo "C4: Found TODO: remove markers - please clean up before next session" >&2
fi
exit 0
