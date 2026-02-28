#!/bin/bash
# c9-notify.sh: C9 Research Loop 알림 발송
#   - Dooray incoming webhook (state.yaml notify.dooray_webhook)
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
import yaml, socket, os
s = yaml.safe_load(open(os.environ['STATE_FILE']))
n = s.get('notify', {})
r = os.environ.get('ROUND_ARG') or str(s.get('round', 0))
t = n.get('templates', {})
vals = [
    n.get('dooray_webhook', ''),
    n.get('session', ''),
    n.get('bot_name', 'C9 Lab'),
    n.get('server_id', socket.gethostname()),
    r,
    t.get('dooray', '{emoji} **[C9 R{round} · {phase}]** [{server}] {message}'),
    t.get('mail', '[C9-{phase}] Round={round} server={server} {message}'),
]
print('\n'.join(vals))
PYEOF
) 2>/dev/null

DOORAY_WEBHOOK=$(echo "$_CONFIG" | sed -n '1p')
SESSION_NAME=$(echo  "$_CONFIG" | sed -n '2p')
BOT_NAME=$(echo      "$_CONFIG" | sed -n '3p')
SERVER_ID=$(echo     "$_CONFIG" | sed -n '4p')
ROUND=$(echo         "$_CONFIG" | sed -n '5p')
TMPL_DOORAY=$(echo   "$_CONFIG" | sed -n '6p')
TMPL_MAIL=$(echo     "$_CONFIG" | sed -n '7p')

# fallback defaults
ROUND="${ROUND:-0}"
BOT_NAME="${BOT_NAME:-C9 Lab}"
SERVER_ID="${SERVER_ID:-$(hostname)}"
TMPL_DOORAY="${TMPL_DOORAY:-{emoji} **[C9 R{round} · {phase}]** [{server}] {message}}"
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

# ── 템플릿 렌더링 ─────────────────────────────────────────────
render_template() {
    local tmpl="$1"
    local safe_msg safe_server
    safe_msg=$(printf '%s' "$MESSAGE" | sed 's/[&|\\]/\\&/g')
    safe_server=$(printf '%s' "$SERVER_ID" | sed 's/[&|\\]/\\&/g')
    echo "$tmpl" \
        | sed "s|{emoji}|${EMOJI}|g" \
        | sed "s|{phase}|${PHASE}|g" \
        | sed "s|{round}|${ROUND}|g" \
        | sed "s|{server}|${safe_server}|g" \
        | sed "s|{message}|${safe_msg}|g" \
        | sed "s|{timestamp}|${TIMESTAMP}|g"
}

# ── Dooray 발송 ──────────────────────────────────────────────
if [[ -n "$DOORAY_WEBHOOK" ]]; then
    DOORAY_TEXT=$(render_template "$TMPL_DOORAY")
    if [[ -n "$METRICS" ]]; then
        DOORAY_TEXT="${DOORAY_TEXT}\n\`\`\`\n${METRICS}\n\`\`\`"
    fi
    DOORAY_TEXT="${DOORAY_TEXT}\n_${TIMESTAMP}_"

    DOORAY_PAYLOAD=$(python3 -c "
import json
text = '''$DOORAY_TEXT'''
print(json.dumps({'botName': '$BOT_NAME', 'text': text}))
")

    curl -s -X POST "$DOORAY_WEBHOOK" \
        -H "Content-Type: application/json" \
        -d "$DOORAY_PAYLOAD" \
        -o /dev/null \
        && echo "[c9-notify] Dooray OK" \
        || echo "[c9-notify] Dooray failed (non-critical)"
fi

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
