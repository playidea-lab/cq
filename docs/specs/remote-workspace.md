# Spec: Remote Workspace

## Feature
원격 워커를 로컬처럼 자유롭게 조작하는 MCP 도구 2개.

## Domain
infra / relay / mcp-handlers

## Requirements (EARS)

- R1 [Ubiquitous]: 시스템은 relay에 연결된 모든 워커의 목록과 상태를 MCP 도구(cq_workers)로 제공해야 한다
- R2 [Ubiquitous]: 시스템은 원격 워커에 등록된 아무 MCP 도구를 로컬에서 호출(cq_relay_call)할 수 있어야 한다
- R3 [Event-driven]: 워커가 오프라인이면 시스템은 "worker offline" 에러를 즉시 반환해야 한다
- R4 [State-driven]: relay 연결이 없는 상태에서 원격 도구 호출 시 시스템은 설정 안내를 반환해야 한다
- R5 [Unwanted]: 1MB 이상의 응답은 relay를 통해 전송되어서는 안 된다
- R7 [Event-driven]: 원격 실행 응답이 30초 내 오지 않으면 timeout 에러와 함께 cq hub submit 안내를 반환해야 한다

## Non-Functional
- relay /health 호출은 10초 timeout
- relay /w/{id}/mcp 프록시는 30초 timeout
- 응답 크기 1MB 초과 시 truncate + Drive 안내

## Out of Scope
- cq transfer --from (역방향 전송)
- relay HA (다중 machine)
- 토큰 자동 refresh (별도 이슈)
