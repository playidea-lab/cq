#!/bin/bash
# C4 Gate Hook (PreToolUse)
# Rule-based fast gate for Bash/Edit/Write tools.
# Reads allow/block patterns from $CLAUDE_PROJECT_DIR/.c4/hook-config.json.
# Gray-zone commands (no pattern match) exit 0 — passed to PermissionRequest hook.
#
# Exit codes:
#   0 - No decision (pass to PermissionRequest, or allow if no further hook)
#   2 - Block command (deny)
#
# Protocol output (stdout):
#   allow: {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow",...}}
#   deny:  {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",...}}

# =============================================================================
# Read input from stdin
# =============================================================================
INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null)
# For Bash: use command; for Edit/Write: use file_path
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null)

# Use CLAUDE_PROJECT_DIR if set; otherwise walk up from PWD
_find_c4_root() {
    if [[ -n "${CLAUDE_PROJECT_DIR:-}" ]] && [[ -d "${CLAUDE_PROJECT_DIR}/.c4" ]]; then
        echo "$CLAUDE_PROJECT_DIR"
        return 0
    fi
    local dir="$PWD"
    while [[ "$dir" != "/" ]] && [[ -n "$dir" ]]; do
        if [[ -d "$dir/.c4" ]]; then
            echo "$dir"
            return 0
        fi
        dir="${dir%/*}"
    done
    return 1
}

C4_ROOT=$(_find_c4_root) || exit 0

# Load config
HOOK_CONFIG_JSON="${C4_ROOT}/.c4/hook-config.json"
ALLOW_PATTERNS=()
BLOCK_PATTERNS=()

if [[ -f "$HOOK_CONFIG_JSON" ]] && command -v jq &>/dev/null; then
    _enabled=$(jq -r '.enabled // true' "$HOOK_CONFIG_JSON" 2>/dev/null)
    if [[ "$_enabled" == "false" ]]; then
        exit 0
    fi
    while IFS= read -r _p; do [[ -n "$_p" ]] && ALLOW_PATTERNS+=("$_p"); done < <(jq -r '.allow_patterns[]? // empty' "$HOOK_CONFIG_JSON" 2>/dev/null)
    while IFS= read -r _p; do [[ -n "$_p" ]] && BLOCK_PATTERNS+=("$_p"); done < <(jq -r '.block_patterns[]? // empty' "$HOOK_CONFIG_JSON" 2>/dev/null)
else
    # No config: C4 project exists but MCP not started yet — use built-in safe allow patterns
    ALLOW_PATTERNS=(
        "^git (log|show|diff|status|branch|tag|remote|stash list|ls-files|blame)( |$)"
        "^go (build|test|vet|run|env|list|mod)( |$)"
        "^uv (run|sync|pip list|pip show|lock|python)( |$)"
        "^(ls|ll|la|cat|head|tail|wc|stat)( |$)"
        "^find \\. "
        "^(grep|rg) "
        "^(echo|pwd|which|env|printenv|date|uname|whoami)( |$)"
        "^(cargo|rustc) (build|test|check|clippy)( |$)"
        "^pnpm (build|test|lint|install|dev)( |$)"
        "^cq (status|doctor|mcp|version|help)( |$)"
        "^sqlite3 "
    )
fi

# =============================================================================
# Helper emit functions
# =============================================================================
_emit_allow() {
    local reason="${1:-Safe}"
    jq -n --arg reason "$reason" '{
        hookSpecificOutput: {
            hookEventName: "PreToolUse",
            permissionDecision: "allow",
            permissionDecisionReason: $reason
        }
    }' 2>/dev/null || {
        local _r="${reason//\\/\\\\}"; _r="${_r//\"/\\\"}"
        echo "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"allow\",\"permissionDecisionReason\":\"$_r\"}}"
    }
    exit 0
}

_emit_deny() {
    local reason="${1:-Blocked}"
    jq -n --arg reason "$reason" '{
        hookSpecificOutput: {
            hookEventName: "PreToolUse",
            permissionDecision: "deny",
            permissionDecisionReason: $reason
        }
    }' 2>/dev/null || {
        local _r="${reason//\\/\\\\}"; _r="${_r//\"/\\\"}"
        echo "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"$_r\"}}"
    }
    exit 2
}

# =============================================================================
# C4 프로젝트 전용: MCP 내장 도구 차단 (TodoWrite / TaskCreate / TaskUpdate)
# EnterPlanMode는 /pi 스킬(ideation 모드)에서 합법적으로 사용 — 차단하지 않음
# tool_name은 "TodoWrite" 또는 MCP 네임스페이스 형식 "mcp__<ns>__TodoWrite" 두 가지 모두 처리
# =============================================================================

# Normalize: mcp__*__ToolName → ToolName (마지막 __ 구분자 이후 추출)
_BARE_TOOL="$TOOL_NAME"
if [[ "$TOOL_NAME" =~ ^mcp__.*__(.+)$ ]]; then
    _BARE_TOOL="${BASH_REMATCH[1]}"
fi

if [[ "$_BARE_TOOL" == "TodoWrite" ]]; then
    _emit_deny "TodoWrite 금지 (C4 프로젝트). c4_add_todo 또는 /c4-add-task 스킬 사용"
fi

if [[ "$_BARE_TOOL" == "TaskCreate" ]]; then
    _emit_deny "TaskCreate 금지 (C4 프로젝트). c4_add_todo 사용 (단일 소스: .c4/tasks.db)"
fi

if [[ "$_BARE_TOOL" == "TaskUpdate" ]]; then
    _emit_deny "TaskUpdate 금지 (C4 프로젝트). c4_task_list 또는 c4_status로 확인 후 c4_submit 사용"
fi

# EnterPlanMode: /pi 스킬(ideation 모드)에서 허용 — 블록 제거
# 구현 계획 수립 목적(c4-plan 대체)으로 직접 호출 시는 /c4-plan 사용할 것

# =============================================================================
# C4 워크플로우 게이트 (c4-finish 및 git commit 순서 강제)
# =============================================================================
_SKILL_NAME=$(echo "$INPUT" | jq -r '.tool_input.skill // empty' 2>/dev/null)
_DB_PATH="${DB_PATH:-${C4_ROOT}/.c4/c4.db}"

# 인라인 우회: C4_SKIP_GATE=1 (export 금지 — 세션 전체 bypass 방지)
_GATES_ENABLED=1
[[ -n "${C4_SKIP_GATE:-}" ]] && _GATES_ENABLED=0

# c4_gates 테이블 존재 여부 확인 (없으면 skip)
if [[ "$_GATES_ENABLED" == 1 ]]; then
    if [[ -f "$_DB_PATH" ]] && command -v sqlite3 &>/dev/null; then
        _cnt=$(sqlite3 "$_DB_PATH" \
            "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='c4_gates'" 2>/dev/null)
        [[ "${_cnt:-0}" -eq 0 ]] && _GATES_ENABLED=0
    else
        _GATES_ENABLED=0
    fi
fi

# _c4_polish_done: 현재 배치의 polish 게이트가 완료되었는지 확인
_c4_polish_done() {
    # 현재 배치 시작 시점 = active 태스크 중 가장 최근 created_at
    local _batch_start
    _batch_start=$(sqlite3 "$_DB_PATH" \
        "SELECT MAX(created_at) FROM c4_tasks WHERE status IN ('pending','in_progress')" 2>/dev/null)
    if [[ -z "$_batch_start" || "$_batch_start" == "NULL" ]]; then
        echo "no_active"
        return
    fi
    sqlite3 "$_DB_PATH" \
        "SELECT 1 FROM c4_gates WHERE gate='polish' AND status IN ('done','skipped')
         AND completed_at >= '${_batch_start}' LIMIT 1" 2>/dev/null
}

# BLOCK A: Skill c4-finish 인터셉트
if [[ "$_BARE_TOOL" == "Skill" && "$_SKILL_NAME" == "c4-finish" ]]; then
    if [[ "$_GATES_ENABLED" == 1 ]]; then
        _pdone=$(_c4_polish_done)
        if [[ -z "$_pdone" ]]; then
            _emit_deny "c4-finish 차단: /c4-polish 먼저 실행 필요. 우회(inline only): C4_SKIP_GATE=1 /c4-finish (export 금지)"
        fi
    fi
fi

# BLOCK B: git commit 가드 (진행 중 태스크 + polish 미완료 시 차단)
if [[ "$TOOL_NAME" == "Bash" && "$COMMAND" =~ ^git[[:space:]]+commit ]]; then
    if [[ "$_GATES_ENABLED" == 1 ]]; then
        _in_prog=$(sqlite3 "$_DB_PATH" \
            "SELECT COUNT(*) FROM c4_tasks WHERE status='in_progress'" 2>/dev/null)
        if [[ "${_in_prog:-0}" -gt 0 ]]; then
            _pdone=$(_c4_polish_done)
            if [[ -z "$_pdone" ]]; then
                _emit_deny "git commit 차단: 진행 중 태스크 ${_in_prog}개 + polish 미완료. 우회(inline only): C4_SKIP_GATE=1 git commit -m '...'"
            fi
        fi
    fi
fi

# =============================================================================
# Bash tool: check allow/block patterns against command
# =============================================================================
if [[ "$TOOL_NAME" == "Bash" ]] && [[ -n "$COMMAND" ]]; then
    # Allow patterns — user whitelist
    for pattern in "${ALLOW_PATTERNS[@]}"; do
        if [[ "$COMMAND" =~ $pattern ]]; then
            _emit_allow "Allow pattern match"
        fi
    done

    # Block patterns — user blacklist
    for pattern in "${BLOCK_PATTERNS[@]}"; do
        if [[ "$COMMAND" =~ $pattern ]]; then
            _emit_deny "Blocked by config: matches '$pattern'"
        fi
    done

    # Built-in dangerous patterns — always block
    # System destruction
    if [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+/ ]] || \
       [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+~ ]] || \
       [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+\* ]]; then
        _emit_deny "Recursive delete of system/home directory"
    fi

    # Git force push to main/master
    if [[ "$COMMAND" =~ git[[:space:]]+push[[:space:]].*--force-with-lease ]]; then
        : # --force-with-lease is safe
    elif [[ "$COMMAND" =~ git[[:space:]]+push[[:space:]]+(-f|--force)[[:space:]]+(origin[[:space:]]+)?(main|master) ]] || \
         [[ "$COMMAND" =~ git[[:space:]]+push[[:space:]]+(origin[[:space:]]+)?(main|master)[[:space:]]+(-f|--force) ]]; then
        _emit_deny "Force push to main/master branch"
    fi

    # Remote code execution via curl/wget pipe to shell (always dangerous)
    if [[ "$COMMAND" =~ (curl|wget)[[:space:]].*\|[[:space:]]*(sh|bash|zsh)([[:space:]]|$) ]]; then
        _emit_deny "Piping remote content to shell"
    fi

    # Piping to interpreter with NO local code: | python3 (bare) → stdin이 코드로 실행됨
    # Allow: | python3 -c "...", | python3 script.py, | python -m json.tool
    if [[ "$COMMAND" =~ (curl|wget)[[:space:]].*\|[[:space:]]*(python[0-9.]*|perl|ruby)[[:space:]]*$ ]]; then
        _emit_deny "Piping remote content to bare interpreter (stdin executed as code). Use -c or a local script."
    fi
fi

# =============================================================================
# Edit/Write tool: check patterns against file_path
# Priority: built-in allow > user allow > user block > built-in block
# =============================================================================
if [[ "$TOOL_NAME" == "Edit" || "$TOOL_NAME" == "Write" ]] && [[ -n "$FILE_PATH" ]]; then
    # Built-in allow: .c4/ system files and /tmp/ — checked first so user block patterns can't override
    if [[ "$FILE_PATH" =~ (^|/)\.c4/ ]] || [[ "$FILE_PATH" =~ ^/tmp/ ]]; then
        _emit_allow "C4 system or temp file"
    fi

    # User allow patterns — checked before user block patterns
    for pattern in "${ALLOW_PATTERNS[@]}"; do
        if [[ "$FILE_PATH" =~ $pattern ]]; then
            _emit_allow "Allow pattern match"
        fi
    done

    # User block patterns
    for pattern in "${BLOCK_PATTERNS[@]}"; do
        if [[ "$FILE_PATH" =~ $pattern ]]; then
            _emit_deny "Blocked by config: matches '$pattern'"
        fi
    done

    # Built-in block: system directories
    if [[ "$FILE_PATH" =~ ^/(etc|usr/bin|usr/local/bin|bin|sbin)/ ]]; then
        _emit_deny "Writing to system directory: $FILE_PATH"
    fi
fi

# No pattern matched — exit 0 (gray zone, let PermissionRequest handle it)
exit 0
