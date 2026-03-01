# C9 State Schema

> `.c9/state.yaml` is the single source of truth for C9 research loop configuration.

## Schema Reference

```yaml
# Required
phase: string          # CONFERENCE | IMPLEMENT | RUN | CHECK | REFINE | FINISH | DONE
round: int             # current round number (0 = baseline)

# Project metadata
project:
  name: string                  # short project identifier (e.g., "unified-hmr")
  goal: string                  # one-sentence research goal

# Primary metric (domain-neutral)
metric:
  name: string                  # metric identifier (MPJPE, F1, BLEU, accuracy, etc.)
  unit: string | null           # display unit (e.g., mm, %, score) — used in logs/notifications
  lower_is_better: bool         # true for error metrics (MPJPE), false for accuracy (F1)
  convergence_threshold: float  # minimum improvement to continue loop
  baseline: float | null        # initial metric value (set after round 0)

# Hub connection
hub:
  url: string                   # Hub API endpoint
  # NOTE: api_key is NEVER stored here. Use `cq secret set hub.api_key`

# Notifications (all optional)
notify:
  dooray_webhook: string | null
  session: string | null        # cq session name for c4_mail_send
  bot_name: string
  server_id: string | null
  templates:
    dooray: string              # supports {emoji}, {round}, {phase}, {server}, {message}
    mail: string

# Runtime state
active_jobs: list               # currently running job IDs
max_rounds: int                 # safety limit
last_check: string | null       # ISO 8601 timestamp of last CHECK phase
steer_reason: string | null     # reason for last manual phase override (set by /c9-steer)

# Experiment history (append-only)
metric_history:
  - round: int
    best_exp: string            # experiment ID
    value: float                # primary metric value
    pa_value: float | null      # optional secondary metric
    improvement: float | null   # delta vs previous round
    note: string                # human-readable summary

# Completion state
finish:
  best_round: int | null
  best_value: float | null
  best_model_path: string | null
  artifact_path: string | null
  completed_at: string | null   # ISO 8601
```

## Security

**API keys must NEVER appear in `state.yaml`.**

Use the CQ secret store:

```bash
cq secret set c9.hub.api_key <value>
```

Scripts should resolve keys via `cq secret get c9.hub.api_key` or the `C9_API_KEY` environment variable.

## Migration Guide

### From legacy HMR-specific format

The following fields have been renamed to be domain-neutral:

| Legacy field | New field |
|---|---|
| `convergence_threshold_mm` | `metric.convergence_threshold` |
| `mpjpe_history` | `metric_history` |
| `mpjpe_history[].best_mpjpe` | `metric_history[].value` |
| `mpjpe_history[].pa_mpjpe` | `metric_history[].pa_value` |

### Migration script (python3 + PyYAML)

```python
#!/usr/bin/env python3
"""Migrate legacy .c9/state.yaml to domain-neutral schema."""
import os
import sys
import tempfile
import yaml

def migrate(path: str) -> None:
    with open(path) as f:
        state = yaml.safe_load(f)

    changed = False

    # convergence_threshold_mm → metric.convergence_threshold
    if "convergence_threshold_mm" in state:
        if "metric" not in state:
            state["metric"] = {}
        state["metric"]["convergence_threshold"] = state.pop("convergence_threshold_mm")
        changed = True

    # mpjpe_history → metric_history
    if "mpjpe_history" in state:
        old_hist = state.pop("mpjpe_history")
        new_hist = []
        for entry in old_hist:
            new_entry = {"round": entry.get("round", 0)}
            if "exp" in entry:
                new_entry["best_exp"] = entry["exp"]
            elif "best_exp" in entry:
                new_entry["best_exp"] = entry["best_exp"]
            if "best_mpjpe" in entry:
                new_entry["value"] = entry["best_mpjpe"]
            elif "value" in entry:
                new_entry["value"] = entry["value"]
            if "pa_mpjpe" in entry:
                new_entry["pa_value"] = entry["pa_mpjpe"]
            new_entry["improvement"] = entry.get("improvement")
            new_entry["note"] = entry.get("note", "")
            new_hist.append(new_entry)
        state["metric_history"] = new_hist
        changed = True

    if not changed:
        print("No migration needed.")
        return

    # Atomic write: NamedTemporaryFile → os.replace
    # NEVER use sed for YAML — it can corrupt structure
    dirpath = os.path.dirname(os.path.abspath(path))
    with tempfile.NamedTemporaryFile(
        mode="w", dir=dirpath, suffix=".yaml", delete=False
    ) as tmp:
        yaml.dump(state, tmp, default_flow_style=False, allow_unicode=True)
        tmp_path = tmp.name
    os.replace(tmp_path, path)
    print(f"Migrated: {path}")

if __name__ == "__main__":
    migrate(sys.argv[1] if len(sys.argv) > 1 else ".c9/state.yaml")
```

### Usage

```bash
# Backup first
cp .c9/state.yaml .c9/state.yaml.bak

# Run migration
python3 migrate_c9_state.py .c9/state.yaml

# Verify
python3 -c "import yaml; s=yaml.safe_load(open('.c9/state.yaml')); assert 'metric_history' in s"
```

### Important notes

- Use `python3` with PyYAML for all state.yaml modifications
- Always use atomic writes (`NamedTemporaryFile` + `os.replace`)
- Never use `sed` on YAML files (risk of structural corruption)
- The `api_key` field must never appear in `state.yaml` (use `cq secret set`)
