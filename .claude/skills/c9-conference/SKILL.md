# C9 Conference

Claude(Opus) + Gemini(Pro)가 합의에 이를 때까지 토론하고 결과를 사용자에게 보고하는 리서치 컨퍼런스 모드.

**트리거**: "c9-conference", "컨퍼런스", "둘이 토론해서", "합의 나와봐"

## 실행 순서

### Step 1: 주제 설정 + Knowledge 조회 (plan 패턴)
- 사용자 입력에서 토론 주제 추출
- 관련 실험 컨텍스트 있으면 수집 (이전 c9-report 결과 등)
- **Knowledge DB 조회**:

```
c4_pattern_suggest(context="{토론 주제}")
c4_experiment_search(query="{토론 주제}")
```

→ 결과가 있으면 Claude 첫 턴에 "## 과거 패턴/실험" 섹션 주입
→ 결과가 없으면 건너뜁니다.

**쓰기는 c9-finish에 위임** — conference 합의문은 c9-finish가 연구 루프 완료 시 기록.

### Step 2: 컨퍼런스 루프 (최대 5라운드)

각 라운드:

**[Claude 턴]**
- 현재 포지션 명시: `POSITION: ...`
- 핵심 주장 (2문단, 메커니즘 기반)
- 이전 라운드 Gemini의 OPEN에 응답

**[Gemini 턴]** — `scripts/c9-conference.sh` 호출:
```bash
echo "<컨텍스트>" | ./scripts/c9-conference.sh "<Claude 턴 전문>"
```

**수렴 판정**:
- Gemini 응답에 `CONSENSUS:` 있으면 → 합의 성립
- 양쪽 모두 상대방 POSITION을 수용하면 → 합의 성립
- 5라운드 후에도 미합의 → "UNRESOLVED: [핵심 쟁점]" 으로 종료

### Step 3: 합의문 출력

```
## C9 Conference 결과

**주제**: ...
**라운드**: N회

**합의 내용**:
- [Claude 양보 사항]
- [Gemini 양보 사항]
- [공동 결론]

**다음 실험 제안** (합의 기반):
1. expXXX: ...
2. expXXX: ...

**미합의 사항** (있는 경우):
- [남은 쟁점]
```

### Step 4: 사용자 승인 대기
합의문을 제시하고 실험 진행 여부 확인.

## 출력 형식
- 각 라운드는 `[Round N]` 헤더로 구분
- Claude 턴은 일반 텍스트, Gemini 턴은 인용 블록(`>`)
- 최종 합의문은 코드 블록

## 주의
- Gemini 응답의 MCP 에러 메시지는 무시 (정상 노이즈)
- `CONSENSUS:` 키워드가 나오면 즉시 루프 종료
- 컨텍스트가 길면 핵심 수치만 추려서 `c9-conference.sh`에 전달
