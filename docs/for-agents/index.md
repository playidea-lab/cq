# For AI Agents

This section provides machine-readable documentation for AI agents and LLMs.

## Available formats

### `llms.txt`

A single plain-text file following the [llms.txt standard](https://llmstxt.org).
Optimized for LLM consumption — concise, structured, no noise.

```
GET https://playidealab.github.io/cq/llms.txt
```

### `skills.jsonl`

One JSON object per line, describing each CQ skill:

```
GET https://playidealab.github.io/cq/api/skills.jsonl
```

### `tools.jsonl`

One JSON object per line, describing each MCP tool:

```
GET https://playidealab.github.io/cq/api/tools.jsonl
```

## Usage example

```python
import httpx, json

skills = [
    json.loads(line)
    for line in httpx.get("https://playidealab.github.io/cq/api/skills.jsonl").text.splitlines()
    if line.strip()
]
```

## Schema

### skills.jsonl

```json
{
  "name": "c4-plan",
  "trigger": ["/c4-plan", "계획", "plan", "설계", "기획"],
  "description": "Discovery → Design → Lighthouse contracts → Task creation. Full structured plan.",
  "tier": "all",
  "category": "workflow"
}
```

### tools.jsonl

```json
{
  "name": "c4_status",
  "description": "Get current C4 project status including state, queue, and workers",
  "tier": "all",
  "category": "status"
}
```
