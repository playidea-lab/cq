#!/usr/bin/env python3
"""
C4 PermissionRequest Hook: LLM-based auto-judgment for gray-zone permissions.

Reads settings from .c4/config.yaml (permission_reviewer section):
  enabled: true/false     — master switch (default: false)
  model: haiku/sonnet     — which Claude model to use (default: haiku)
  api_key_env: ENV_NAME   — env var for API key (default: ANTHROPIC_API_KEY)
  fail_mode: ask/allow    — behavior on API failure (default: ask)
  timeout: 10             — API call timeout in seconds (default: 10)

AskUserQuestion excluded from matcher — business decisions stay with human.
"""

import json
import os
import subprocess
import sys

# Model ID mapping
MODEL_IDS = {
    "haiku": "claude-haiku-4-5-20251001",
    "sonnet": "claude-sonnet-4-5-20250929",
    "opus": "claude-opus-4-6",
}

def load_rules():
    """Load permission rules from permission-rules.md next to this script."""
    rules_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "permission-rules.md")
    try:
        with open(rules_path) as f:
            return f.read().strip()
    except Exception:
        return (
            "You are a permission reviewer. APPROVE safe dev operations. "
            "DENY destructive or irreversible actions. "
            'Respond ONLY with JSON: {"ok": true} or {"ok": false, "reason": "..."}.'
        )

# Defaults (matching Go config defaults)
DEFAULT_CONFIG = {
    "enabled": False,
    "model": "haiku",
    "api_key_env": "ANTHROPIC_API_KEY",
    "fail_mode": "ask",
    "timeout": 10,
}


def load_config():
    """Load permission_reviewer config from .c4/config.yaml."""
    cfg = dict(DEFAULT_CONFIG)
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR", "")
    if not project_dir:
        return cfg
    config_path = os.path.join(project_dir, ".c4", "config.yaml")
    if not os.path.isfile(config_path):
        return cfg
    try:
        # Minimal YAML parsing — avoid external deps
        with open(config_path) as f:
            in_section = False
            for line in f:
                stripped = line.strip()
                if stripped.startswith("#") or not stripped:
                    continue
                # Detect top-level key (no indentation)
                if not line[0].isspace() and stripped.endswith(":"):
                    in_section = stripped == "permission_reviewer:"
                    continue
                if not line[0].isspace() and ":" in stripped:
                    in_section = stripped.startswith("permission_reviewer:")
                    if in_section and not stripped.endswith(":"):
                        # Single-line: permission_reviewer: {inline} — skip
                        pass
                    continue
                if in_section and line[0].isspace() and ":" in stripped:
                    key, _, val = stripped.partition(":")
                    key = key.strip()
                    val = val.strip().strip("\"'")
                    if key in cfg:
                        if key == "enabled":
                            cfg[key] = val.lower() in ("true", "yes", "1")
                        elif key == "timeout":
                            try:
                                cfg[key] = int(val)
                            except ValueError:
                                pass
                        else:
                            cfg[key] = val
    except Exception:
        pass
    return cfg


def load_api_key(env_name):
    """Load API key from environment, .env file, or login shell."""
    key = os.environ.get(env_name)
    if key:
        return key
    # Try project .env file
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR", "")
    if project_dir:
        env_path = os.path.join(project_dir, ".env")
        if os.path.isfile(env_path):
            try:
                with open(env_path) as f:
                    for line in f:
                        line = line.strip()
                        if line.startswith(f"{env_name}="):
                            val = line.split("=", 1)[1].strip().strip("\"'")
                            if val:
                                return val
            except Exception:
                pass
    # Try login shell
    try:
        result = subprocess.run(
            ["zsh", "-lc", f"echo ${env_name}"],
            capture_output=True,
            text=True,
            timeout=3,
        )
        key = result.stdout.strip()
        if key:
            return key
    except Exception:
        pass
    return None


def call_model(request_info, api_key, model_name, timeout):
    """Call Anthropic API to review the permission request."""
    model_id = MODEL_IDS.get(model_name, MODEL_IDS["haiku"])
    payload = json.dumps({
        "model": model_id,
        "max_tokens": 100,
        "messages": [{"role": "user", "content": request_info}],
        "system": load_rules(),
    })
    try:
        result = subprocess.run(
            [
                "curl",
                "-s",
                "--max-time",
                str(timeout),
                "https://api.anthropic.com/v1/messages",
                "-H",
                "content-type: application/json",
                "-H",
                f"x-api-key: {api_key}",
                "-H",
                "anthropic-version: 2023-06-01",
                "-d",
                payload,
            ],
            capture_output=True,
            text=True,
            timeout=timeout + 3,
        )
        resp = json.loads(result.stdout)
        text = resp["content"][0]["text"]
        start = text.find("{")
        end = text.rfind("}") + 1
        if start >= 0 and end > start:
            return json.loads(text[start:end])
        return None
    except Exception:
        return None


def emit(behavior, message=""):
    output = {
        "hookSpecificOutput": {
            "hookEventName": "PermissionRequest",
            "decision": {"behavior": behavior},
        }
    }
    if behavior == "deny" and message:
        output["hookSpecificOutput"]["decision"]["message"] = message
    sys.stdout.write(json.dumps(output))


def main():
    cfg = load_config()

    # Master switch — if disabled, don't emit (user gets normal prompt)
    if not cfg["enabled"]:
        return

    payload = json.load(sys.stdin)
    tool_name = payload.get("tool_name", "unknown")
    tool_input = payload.get("tool_input", {})

    if tool_name == "Bash":
        request_info = f"Tool: Bash\nCommand: {tool_input.get('command', '')}"
    elif tool_name in ("Edit", "Write", "MultiEdit"):
        request_info = f"Tool: {tool_name}\nFile: {tool_input.get('file_path', '')}"
    elif tool_name == "Read":
        request_info = f"Tool: Read\nFile: {tool_input.get('file_path', '')}"
    elif tool_name == "WebFetch":
        request_info = f"Tool: WebFetch\nURL: {tool_input.get('url', '')}"
    else:
        request_info = (
            f"Tool: {tool_name}\n"
            f"Input: {json.dumps(tool_input, ensure_ascii=False)[:500]}"
        )

    api_key = load_api_key(cfg["api_key_env"])
    if not api_key:
        if cfg["fail_mode"] == "allow":
            emit("allow")
        # else: don't emit → user gets prompted (fail-ask)
        return

    result = call_model(request_info, api_key, cfg["model"], cfg["timeout"])
    if result is None:
        if cfg["fail_mode"] == "allow":
            emit("allow")
        # else: don't emit → user gets prompted (fail-ask)
        return

    if result.get("ok", True):
        emit("allow")
    else:
        emit("deny", result.get("reason", "Permission reviewer denied this request"))


if __name__ == "__main__":
    main()
