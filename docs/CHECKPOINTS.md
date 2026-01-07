# C4 Refactoring Checkpoints

## CP-R1: Breaking Change 검증 ⏳

### Gate Conditions

- [ ] T-R01: 패키지 리네임 c4d → c4
- [ ] T-R02: daemon/ 서브패키지 추출

### Required Validations

- lint: `uv run ruff check c4/ tests/`
- unit: `uv run pytest tests/ -v`

### Decision Criteria

- 127개 테스트 전체 통과
- 모든 import문 수정 완료
- MCP Server 재연결 성공

---

## CP-R2: 최종 검증 ⏳

### Gate Conditions

- [ ] T-R03: models/ 분리
- [ ] T-R04: 테스트 재편성

### Required Validations

- lint
- unit

### Decision Criteria

- 127개+ 테스트 통과
- 최대 파일 LOC < 300
- 테스트 구조 unit/integration/e2e 분리 완료
