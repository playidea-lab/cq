# C4D Review Checkpoints

## CP1: 코드 리뷰 완료
### Gate Conditions
- [ ] T-001: MCP Server 리뷰 완료
- [ ] T-002: State Machine 리뷰 완료
- [ ] T-003: Supervisor 리뷰 완료

### Required Validations
- lint: `uv run ruff check c4d/`
- unit: `uv run pytest tests/`

### Decision Criteria
- 모든 리뷰 태스크 완료
- 테스트 전체 통과

---

## CP2: 프로젝트 완료
### Gate Conditions
- [ ] T-004: 문서 리뷰 완료
- [ ] T-005: README 업데이트 완료

### Required Validations
- lint
- unit

### Decision Criteria
- 전체 리뷰 완료
- 문서화 완료
