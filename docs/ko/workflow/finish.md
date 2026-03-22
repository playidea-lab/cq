# /c4-finish

트리거: `/c4-finish` 또는 키워드: `마무리`, `finish`, `완료`

## 동작 방식

구현 완료 후 실행하는 마무리 루틴입니다.

```
1. Polish 루프 (빌드-테스트-리뷰-수정, 변경 0이 될 때까지, 최대 8라운드)
2. polish 게이트 기록 (c4_record_gate)
3. phase lock 획득 (동시 finish 방지)
4. 빌드 검증 (프로젝트 유형 자동 감지)
5. 전체 테스트 스위트 실행
6. 바이너리 설치 (make install)
7. 세션 지식 기록 (c4_knowledge_record)
8. 모든 변경사항 커밋
9. phase lock 해제
```

polish 루프는 `/c4-run`에서 워커가 사용하는 것과 같은 게이트입니다. 워커든 수동 `/c4-finish`든, Go 레벨 게이트가 수렴을 보장합니다.

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
