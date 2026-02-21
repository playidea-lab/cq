# Skills & Tools JSONL

Structured data for programmatic consumption by AI agents and tooling.

## Skills JSONL

```
GET https://playidealab.github.io/cq/api/skills.jsonl
```

Schema:
```json
{
  "name": "c4-plan",
  "trigger": ["/c4-plan", "계획", "plan", "설계"],
  "description": "Discovery → Design → Tasks. Creates structured implementation plan.",
  "tier": "all"
}
```

## Tools JSONL

```
GET https://playidealab.github.io/cq/api/tools.jsonl
```

Schema:
```json
{
  "name": "c4_status",
  "category": "status",
  "tier": "all",
  "description": "Get project state, queue summary, and worker status."
}
```

## Categories

| Category | Tools |
|----------|-------|
| `status` | c4_status, c4_health |
| `task` | c4_add_todo, c4_get_task, c4_submit, c4_claim, c4_report, c4_task_list |
| `file` | c4_read_file, c4_find_file, c4_search_for_pattern, c4_replace_content |
| `knowledge` | c4_knowledge_search, c4_knowledge_record, c4_knowledge_get |
| `secret` | c4_secret_set, c4_secret_get, c4_secret_list |
| `llm` | c4_llm_call, c4_llm_providers |
| `contract` | c4_lighthouse |

## Usage example

```python
import httpx, json

skills = [
    json.loads(line)
    for line in httpx.get("https://playidealab.github.io/cq/api/skills.jsonl").text.splitlines()
    if line.strip()
]

# Find skill by trigger keyword
def find_skill(keyword):
    return next((s for s in skills if keyword in s['trigger']), None)

find_skill('plan')
# → {"name": "c4-plan", "trigger": [...], "description": "...", "tier": "all"}
```
