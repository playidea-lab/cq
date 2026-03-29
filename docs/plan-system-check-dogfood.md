# 전체 시스템 동작 점검 플랜 (도그푸딩)

## 스펙

- **저장 위치**: `.c4/specs/system-check-dogfood.md`
- **목표**: c4→cq 이름 변경 후 구역별로 시스템 정상 동작 검증

## 구역별 태스크 (8개, 의존성 없음)

| Task ID   | 구역 | 범위 | DoD 요약 |
|-----------|------|------|----------|
| T-CHK-01  | CLI·설치 | install.sh, c4-core/cmd/c4 | cq --version, install.sh, ~/.local/bin/cq |
| T-CHK-02  | MCP 서버 | c4-core/cmd/c4 | initialize + tools/list + c4_status 호출 |
| T-CHK-03  | 설정·경로 | .mcp.json, .cursor/mcp.json | mcpServers.cq, command=cq, 경로 cq |
| T-CHK-04  | 문서 | docs/ | pi/c4 잔여, bin/c4 잔여, cq 문구 통일 |
| T-CHK-05  | Go 코어 | c4-core/ | go build, go vet, cmd/c4 테스트 |
| T-CHK-06  | Python | c4/, pyproject.toml | uv sync, (선택) 사이드카 |
| T-CHK-07  | C1 | c1/ | cargo check / build |
| T-CHK-08  | C5 | c5/ | go build, go test |

## 실행 방법

```bash
# Cursor/Claude Code에서
/run 8
# 또는 자동 병렬
/run
```

- **review_required**: false (점검 태스크는 리뷰 미생성)
- **domain**: check
- Worker가 각 1태스크씩 담당 후 c4_submit(handoff에 결과 요약)

## 상태 확인

```bash
cq status
# 또는 MCP: c4_status
```

완료 후 `c4_status`로 T-CHK-* 8개 done 여부 확인.
