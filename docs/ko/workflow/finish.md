# /c4-finish

트리거: `/c4-finish` 또는 키워드: `마무리`, `finish`, `완료`

## 동작 방식

구현 완료 후 실행하는 마무리 루틴입니다.

```
1. /c4-polish 실행 (리뷰어가 변경 없음 판정할 때까지 수정)
2. phase lock 획득 (동시 finish 방지)
3. 빌드 검증 (프로젝트 유형 자동 감지)
4. 전체 테스트 스위트 실행
5. 문서 업데이트
6. 세션 지식 기록 (c4_knowledge_record)
7. 모든 변경사항 커밋
8. CHANGELOG 생성 (/c4-release 호출)
9. phase lock 해제
```

## 실행 시기

`/c4-run`이 큐가 비워지면 `/c4-finish`를 자동으로 호출합니다 — 보통 수동으로 실행할 필요가 없습니다.

`/c4-run`이 완료된 후 추가 변경을 했거나, 워커 없이 `/c4-plan` + 수동 편집만 한 경우에 수동으로 실행합니다.

## 빌드 검증

CQ가 프로젝트 유형을 자동으로 감지합니다:

| 프로젝트 유형 | 빌드 명령 |
|-------------|---------|
| Go | `go build ./... && go vet ./...` |
| Python | `uv run python -m compileall . && uv run pytest tests/ -x` |
| Node | `npm run build` |
| Rust | `cargo build` |

`.c4/config.yaml`에서 오버라이드:
```yaml
validation:
  build_command: "make build"
  test_command: "make test"
```

## Polish 생략

```
/c4-finish --no-polish    # polish 단계 생략 (긴급 배포용)
```
