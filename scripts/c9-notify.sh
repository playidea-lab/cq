#!/bin/bash
# c9-notify.sh: C9 Research Loop 알림 발송
#   - cq mail (state.yaml notify.session)
#
# Usage:
#   ./scripts/c9-notify.sh <phase> <message> [round] [metrics]
#
# Examples:
#   ./scripts/c9-notify.sh RUN "exp_simvq 훈련 시작" 1
#   ./scripts/c9-notify.sh CHECK "MPJPE=98.3mm (개선 4.3mm)" 1 "mpjpe=98.3,pa=70.1"
#   ./scripts/c9-notify.sh FINISH "연구 완료 Best=96.0mm" 3

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
STATE_FILE="$PROJECT_DIR/.c9/state.yaml"

PHASE="${1:-INFO}"
MESSAGE="${2:-C9 알림}"
METRICS="${4:-}"
TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')

# ── state.yaml에서 notify 설정 한 번에 파싱 ───────────────────
_CONFIG=$(STATE_FILE="$STATE_FILE" ROUND_ARG="${3:-}" python3 - <<'PYEOF'
import yaml, socket, os, json
s = yaml.safe_load(open(os.environ['STATE_FILE']))
n = s.get('notify', {})
r = os.environ.get('ROUND_ARG') or str(s.get('round', 0))
t = n.get('templates', {})
print(json.dumps({
    'session': n.get('session', ''),
    'bot_name': n.get('bot_name', 'C9 Lab'),
    'server_id': n.get('server_id', socket.gethostname()),
    'round': r,
    'tmpl_mail': t.get('mail', '[C9-{phase}] Round={round} server={server} {message}'),
}))
PYEOF
) 2>/dev/null

# JSON에서 키별 추출 (다줄 값 및 특수문자 안전 처리)
SESSION_NAME=$(echo  "$_CONFIG" | python3 -c "import json,sys; print(json.load(sys.stdin).get('session',''))")
BOT_NAME=$(echo      "$_CONFIG" | python3 -c "import json,sys; print(json.load(sys.stdin).get('bot_name','C9 Lab'))")
SERVER_ID=$(echo     "$_CONFIG" | python3 -c "import json,sys; print(json.load(sys.stdin).get('server_id',''))")
ROUND=$(echo         "$_CONFIG" | python3 -c "import json,sys; print(json.load(sys.stdin).get('round','0'))")
TMPL_MAIL=$(echo     "$_CONFIG" | python3 -c "import json,sys; print(json.load(sys.stdin).get('tmpl_mail',''))")

# fallback defaults
ROUND="${ROUND:-0}"
BOT_NAME="${BOT_NAME:-C9 Lab}"
SERVER_ID="${SERVER_ID:-$(hostname)}"
TMPL_MAIL="${TMPL_MAIL:-[C9-{phase}] Round={round} server={server} {message}}"

# ── 단계별 이모지 ─────────────────────────────────────────────
case "$PHASE" in
    CONFERENCE) EMOJI="🧠" ;;
    IMPLEMENT)  EMOJI="🔧" ;;
    RUN)        EMOJI="🚀" ;;
    CHECK)      EMOJI="📊" ;;
    REFINE)     EMOJI="🔄" ;;
    FINISH)     EMOJI="🎉" ;;
    ERROR)      EMOJI="❌" ;;
    BLOCKED)    EMOJI="🚫" ;;
    *)          EMOJI="ℹ️"  ;;
esac

# ── 템플릿 렌더링 (env var 패턴 — sed 구분자 충돌 방지) ──────
render_template() {
    local tmpl="$1"
    C9_TMPL="$tmpl" C9_EMOJI="$EMOJI" C9_PHASE="$PHASE" C9_ROUND="$ROUND" \
    C9_SERVER="$SERVER_ID" C9_MSG="$MESSAGE" C9_TIMESTAMP="$TIMESTAMP" \
    python3 -c "
import os
tmpl = os.environ['C9_TMPL']
for k, v in [
    ('{emoji}', os.environ['C9_EMOJI']),
    ('{phase}', os.environ['C9_PHASE']),
    ('{round}', os.environ['C9_ROUND']),
    ('{server}', os.environ['C9_SERVER']),
    ('{message}', os.environ['C9_MSG']),
    ('{timestamp}', os.environ['C9_TIMESTAMP']),
]:
    tmpl = tmpl.replace(k, v)
print(tmpl, end='')
"
}

# ── cq mail (named session으로) ───────────────────────────────
if [[ -n "$SESSION_NAME" ]]; then
    MAIL_TEXT=$(render_template "$TMPL_MAIL")
    if [[ -n "$METRICS" ]]; then
        MAIL_TEXT="${MAIL_TEXT} | ${METRICS}"
    fi
    cq mail send "$SESSION_NAME" "$MAIL_TEXT" 2>/dev/null \
        && echo "[c9-notify] mail → $SESSION_NAME OK" \
        || echo "[c9-notify] mail failed (non-critical)"
fi

echo "[c9-notify] ${EMOJI} Phase=${PHASE} Round=${ROUND} Server=${SERVER_ID} Message=${MESSAGE}"
