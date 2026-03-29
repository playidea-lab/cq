# Growth Loop

CQ는 당신의 작업 방식을 학습하여 AI 동작을 선호도에 맞게 영구적으로 변경합니다.

## 해결하는 문제

AI 세션은 매번 처음부터 시작됩니다. 당신의 규칙을 설명하고, 같은 실수를 수정하고, 같은 제약사항을 다시 말합니다 — 세션마다. Growth Loop는 이 루프를 닫습니다.

## 동작 방식

```
세션 종료 → 선호도 추출 → 카운트 증가
                                │
                      count ≥ 3 → CLAUDE.md에 힌트 기록
                      count ≥ 5 → .claude/rules/에 규칙으로 승격
                                   AI 동작이 영구적으로 변화
```

네 가지 컴포넌트:

| 컴포넌트 | 역할 |
|---------|------|
| **PreferenceLedger** | 관찰된 선호도를 발생 횟수와 함께 저장 |
| **Hints** | `CLAUDE.md`에 추가되는 제안 (count ≥ 3) |
| **Rules** | `.claude/rules/`의 영구 동작 규칙 (count ≥ 5) |
| **GlobalPromoter** | 패턴을 비개인화하여 커뮤니티 풀에 공유 |

## 실제 사례

5번의 ML 연구 세션 후, CQ가 관찰된 동작에서 자동으로 학습한 패턴:

| 발생 횟수 | 레벨 | CQ가 학습한 것 |
|----------|------|--------------|
| 5x | **Rule** | "Hub를 통해 자동으로 실험 실행" |
| 4x | Hint | "`@key=value` 메트릭 출력 형식 사용" |
| 4x | Hint | "MPJPE/HD/MSD 메트릭 먼저 확인" |
| 3x | Hint | "시작 전 항상 `cq doctor` 실행" |

이 규칙들은 `CLAUDE.md`와 `.claude/rules/`에 주입되어 — 이후 모든 세션의 시스템 프롬프트에 로드됩니다.

## 선호도 → Hint → Rule 생명주기

### 1단계: 관찰

세션이 종료될 때 선호도가 추출됩니다. 출처:
- AI 출력에 대한 수정 사항
- 코드 리뷰 방식의 패턴
- 반복적으로 실행한 커맨드
- 세션 중 명시적으로 제시한 지침

### 2단계: Hint (count ≥ 3)

CQ가 `CLAUDE.md`에 제안을 추가합니다:

```markdown
<!-- CQ hint: count=3 -->
포즈 추정 결과 평가 시 MPJPE/HD/MSD 메트릭을 먼저 확인하세요.
```

Hint는 소프트 가이던스입니다 — AI가 참고하지만 동작을 강제하지는 않습니다.

### 3단계: Rule (count ≥ 5)

CQ가 `.claude/rules/`의 영구 규칙으로 승격합니다:

```markdown
# 자동 생성 규칙 (hint에서 승격됨, count=5)
## 실험 메트릭
항상 MPJPE, HD, MSD 순서로 평가하세요.
결론을 내리기 전에 세 가지 모두를 보고하세요.
```

규칙은 매 세션 시작 시 시스템 프롬프트에 로드됩니다. `CLAUDE.md`의 어떤 내용과도 동등하게 구속력이 있습니다.

### 억제

규칙을 삭제하면 CQ가 영구적으로 억제합니다 — 해당 선호도 패턴에서 다시는 승격되지 않습니다:

```sh
cq rule delete "mpjpe 먼저 확인"    # 영구 억제
```

## PreferenceLedger

Ledger가 선호도 이력을 저장합니다:

```sh
cq preferences list              # 카운트와 함께 모든 선호도
cq preferences list --hints      # Hint만 (count ≥ 3)
cq preferences list --rules      # Rule만 (count ≥ 5)
cq preferences show <id>         # 특정 선호도 상세
```

출력 예시:

```
ID       Count  Level  Preference
pref-01  5      RULE   Hub를 통해 자동으로 실험 실행
pref-02  4      HINT   @key=value 메트릭 출력 형식 사용
pref-03  4      HINT   MPJPE/HD/MSD 메트릭 먼저 확인
pref-04  2      -      커밋 전 임포트 정렬
```

## 지식이 외부로 흐릅니다

선호도는 로컬에만 머물지 않습니다. **GlobalPromoter**가 식별 정보(경로, 사용자명, 이메일, 프로젝트명)를 제거하고 커뮤니티 지식 풀에 패턴을 기여합니다.

공유되는 것:
- 행동 패턴 ("메트릭 X를 Y보다 먼저 확인")
- 워크플로우 순서 ("W 다음에 Z 실행")
- 도구 선호도 ("U 대신 T 사용")

절대 공유되지 않는 것:
- 파일 경로
- 저장소 이름
- 이메일 주소
- 개인 식별자

커뮤니티 패턴은 모든 사용자가 사용할 수 있습니다. 많은 사용자가 독립적으로 같은 모범 사례를 발견하면, 커뮤니티 풀에 노출되어 당신의 시행착오 단계를 건너뛸 수 있게 됩니다.

## 세션과의 연동

Growth Loop는 세션 종료 시 자동으로 트리거됩니다:

```sh
cq session close    # 요약 + 선호도 추출 트리거
```

또는 매 `cq claude` 종료 시 실행되도록 설정할 수 있습니다. 세션에서 추출된 선호도는 Ledger에 병합되고 카운트 증가가 원자적으로 발생합니다.

## Connected와 Full 티어

**connected**와 **full** 티어에서는 PreferenceLedger가 Supabase에 있어 다음에서 공유됩니다:
- 모든 디바이스
- 모든 AI 도구 (Claude Code, ChatGPT, Cursor, Gemini)

Claude Code 세션에서 관찰된 선호도가 다음 날 ChatGPT 세션에서 사용 가능합니다. Growth Loop는 하나의 도구가 아닌 전체 AI 사용에 걸쳐 쌓입니다.

**solo** 티어에서는 Ledger가 로컬 SQLite에 있습니다. 선호도가 쌓이지만 디바이스나 도구 간에 동기화되지는 않습니다.

## 비활성화

세션 종료 시 자동 추출을 비활성화하려면:

```yaml
# .c4/config.yaml
growth_loop:
  enabled: false
```

전체 시스템을 비활성화하지 않고도 개별 규칙을 삭제하고 억제할 수 있습니다.
