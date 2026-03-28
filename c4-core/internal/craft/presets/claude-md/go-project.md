---
name: go-project
description: Go 프로젝트용 CLAUDE.md — 빌드/테스트/린트 명령과 Go 컨벤션
---

# Project Instructions

## Build & Test

```bash
go build ./...
go test ./...
go vet ./...
```

## Code Style

- Error wrapping: `fmt.Errorf("context: %w", err)`
- Named return values only when needed for documentation
- Table-driven tests preferred
- Context as first parameter

# CUSTOMIZE: 프로젝트별 빌드 명령, 디렉토리 구조, 주요 패키지 설명 추가

## Project Structure

<!-- 프로젝트 디렉토리 구조를 여기에 -->

## Key Packages

<!-- 주요 패키지와 역할을 여기에 -->
