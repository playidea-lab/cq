---
name: python-project
description: Python 프로젝트용 CLAUDE.md — uv 기반 빌드/테스트와 Python 컨벤션
---

# Project Instructions

## Build & Test

```bash
uv run pytest
uv run ruff check .
uv run python -m py_compile <file>
```

## CRITICAL: UV Usage

- NEVER: `python script.py`, `pytest`, `pip install`
- ALWAYS: `uv run python script.py`, `uv run pytest`, `uv add <package>`

## Code Style

- Type hints on all public functions
- Pydantic for data validation
- Pathlib instead of os.path
- Context managers for resources

# CUSTOMIZE: 프로젝트 구조, 주요 모듈, 환경 설정 추가

## Project Structure

<!-- 프로젝트 디렉토리 구조를 여기에 -->
