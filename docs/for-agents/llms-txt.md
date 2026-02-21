# llms.txt

CQ publishes a machine-readable summary following the [llms.txt standard](https://llmstxt.org).

## URL

```
https://playidealab.github.io/cq/llms.txt
```

## Usage

```python
import httpx
text = httpx.get("https://playidealab.github.io/cq/llms.txt").text
# Pass as system context to your LLM
```

## Contents

The file includes:
- What CQ is and how it works
- Core concepts (tasks, workers, skills, MCP tools)
- Workflow commands
- Key MCP tools with descriptions
- Task ID grammar
- Security model
- Links to JSONL endpoints

## Preview

```
# CQ — AI Project Orchestration Engine

> CQ is a project management engine for Claude Code...

## Install
## Core Concepts
## Workflow
## Key Skills
## Key MCP Tools
## Task ID Grammar
## Security
## Docs
```
