#!/usr/bin/env python3
"""c9-state-api.py: C5 Research State API helper (G12-B)

Standalone module — no external deps (stdlib only).

Functions:
  get_research_state(hub_url, api_key) -> dict | None
  set_research_state(hub_url, api_key, round, phase, current_version) -> bool

CLI usage (called from shell scripts):
  python c9-state-api.py get <hub_url> <api_key>
  python c9-state-api.py set <hub_url> <api_key> <round> <phase> <version>

Returns:
  get: prints JSON to stdout, exits 0 on success / 1 on failure
  set: exits 0 on success / 2 on 409 conflict / 1 on other failure
"""

import json
import sys
import urllib.request
import urllib.error


_PREFIX = "[c9-state-api]"
_TIMEOUT = 10  # seconds


def get_research_state(hub_url: str, api_key: str) -> "dict | None":
    """GET /v1/research/state — returns {round, phase, version} or None on error."""
    if not hub_url:
        return None
    url = hub_url.rstrip("/") + "/v1/research/state"
    headers = {}
    if api_key:
        headers["X-API-Key"] = api_key
    req = urllib.request.Request(url, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=_TIMEOUT) as resp:
            data = json.loads(resp.read())
            return data
    except urllib.error.HTTPError as e:
        print(f"{_PREFIX} GET failed: HTTP {e.code}", file=sys.stderr)
        return None
    except Exception as e:
        print(f"{_PREFIX} GET error: {e}", file=sys.stderr)
        return None


def set_research_state(
    hub_url: str,
    api_key: str,
    round: int,
    phase: str,
    current_version: int,
) -> bool:
    """PUT /v1/research/state — returns True on success, False on 409 or error."""
    if not hub_url:
        return False
    url = hub_url.rstrip("/") + "/v1/research/state"
    payload = json.dumps(
        {"round": round, "phase": phase, "version": current_version}
    ).encode("utf-8")
    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["X-API-Key"] = api_key
    req = urllib.request.Request(url, data=payload, headers=headers, method="PUT")
    try:
        with urllib.request.urlopen(req, timeout=_TIMEOUT) as resp:
            _ = resp.read()
            return True
    except urllib.error.HTTPError as e:
        if e.code == 409:
            print(
                f"{_PREFIX} 409 Conflict: state version mismatch (another server updated first)",
                file=sys.stderr,
            )
        else:
            print(f"{_PREFIX} PUT failed: HTTP {e.code}", file=sys.stderr)
        return False
    except Exception as e:
        print(f"{_PREFIX} PUT error: {e}", file=sys.stderr)
        return False


def _cmd_get(hub_url: str, api_key: str) -> int:
    """CLI: get — print JSON to stdout."""
    state = get_research_state(hub_url, api_key)
    if state is None:
        return 1
    print(json.dumps(state))
    return 0


def _cmd_set(hub_url: str, api_key: str, round_: int, phase: str, version: int) -> int:
    """CLI: set — return 0=ok, 2=409, 1=error."""
    ok = set_research_state(hub_url, api_key, round_, phase, version)
    if ok:
        return 0
    # Distinguish 409 (already printed) from generic error
    # Re-run to check: we rely on stderr message; exit 2 for 409, 1 otherwise.
    # Since set_research_state already printed the error, just return appropriate code.
    # We use a wrapper that tracks 409 specifically.
    return 1


def _cmd_set_with_409(hub_url: str, api_key: str, round_: int, phase: str, version: int) -> int:
    """CLI: set — return 0=ok, 2=409, 1=error."""
    if not hub_url:
        return 1
    url = hub_url.rstrip("/") + "/v1/research/state"
    payload = json.dumps(
        {"round": round_, "phase": phase, "version": version}
    ).encode("utf-8")
    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["X-API-Key"] = api_key
    req = urllib.request.Request(url, data=payload, headers=headers, method="PUT")
    try:
        with urllib.request.urlopen(req, timeout=_TIMEOUT) as resp:
            _ = resp.read()
            return 0
    except urllib.error.HTTPError as e:
        if e.code == 409:
            print(
                f"{_PREFIX} 409 Conflict: state version mismatch (another server updated first)",
                file=sys.stderr,
            )
            return 2
        print(f"{_PREFIX} PUT failed: HTTP {e.code}", file=sys.stderr)
        return 1
    except Exception as e:
        print(f"{_PREFIX} PUT error: {e}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    args = sys.argv[1:]
    if not args:
        print(f"Usage: {sys.argv[0]} get <hub_url> <api_key>", file=sys.stderr)
        print(f"       {sys.argv[0]} set <hub_url> <api_key> <round> <phase> <version>", file=sys.stderr)
        sys.exit(1)

    cmd = args[0]
    if cmd == "get":
        if len(args) < 3:
            print(f"{_PREFIX} get requires <hub_url> <api_key>", file=sys.stderr)
            sys.exit(1)
        sys.exit(_cmd_get(args[1], args[2]))

    elif cmd == "set":
        if len(args) < 6:
            print(f"{_PREFIX} set requires <hub_url> <api_key> <round> <phase> <version>", file=sys.stderr)
            sys.exit(1)
        try:
            round_ = int(args[3])
            version = int(args[5])
        except ValueError:
            print(f"{_PREFIX} round and version must be integers", file=sys.stderr)
            sys.exit(1)
        sys.exit(_cmd_set_with_409(args[1], args[2], round_, args[4], version))

    else:
        print(f"{_PREFIX} Unknown command: {cmd}", file=sys.stderr)
        sys.exit(1)
