# 빠른 시작

설치부터 첫 결과까지 2분.

## 1. 설치

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## 2. 실행

```sh
cq
```

CQ가 AI 도구 (Claude Code, Cursor, Codex, Gemini)를 자동 감지하고 연결합니다.

## 3. 말하기

그냥 말하세요. CQ가 크기에 맞게 자동 라우팅합니다:

### Small — 직접 수정 (30초)

```
"auth/handler.go 42번 줄 타이포 고쳐줘"
```

CQ가 직접 처리. 계획 없음, Worker 없음. 바로 수정.

### Medium — 퀵 태스크 (2분)

```
/quick "API에 health check 엔드포인트 추가"
```

CQ가 태스크 생성 (DoD 포함) → Worker 스폰 → 결과 제출. 한 커맨드.

### Large — 전체 파이프라인 (5분+)

```
/pi "리트라이 로직이 있는 웹훅 전송 시스템 만들기"
```

CQ가 브레인스토밍 → 계획 → 병렬 Worker → 다듬기 → 커밋. 커피 한 잔.

---

## 끝

4단계는 없습니다. CQ가 각 요청에 맞는 워크플로우를 알아서 선택합니다.

**뒤에서 일어나는 일:**
- 매 세션, CQ가 결정과 선호도를 캡처
- 5번째 세션부터 말하지 않아도 당신의 방식대로
- 지식은 AI 도구를 넘어 흐름 — ChatGPT에서 배운 것이 Claude에서도

---

## 다음은?

| 하고 싶은 것 | 이동 |
|------------|------|
| 4개 기둥 이해 (분산/연결/모방/진화) | [홈](/) |
| 실제 버그 수정 과정 보기 | [버그 수정 예제](/ko/examples/bug-fix) |
| 큰 기능 계획하고 만들기 | [기능 계획](/ko/examples/feature-planning) |
| ChatGPT를 CQ 두뇌에 연결 | [Remote MCP](/ko/examples/remote-mcp) |
| CQ가 선호도 학습하는 과정 보기 | [Growth Loop](/ko/examples/growth-loop-in-action) |
| GPU 실험 자동 반복 | [Research Loop](/ko/examples/research-loop) |

## 문제 해결

```sh
cq doctor    # 뭐가 잘못됐는지 확인
```

| 증상 | 해결 |
|------|------|
| "MCP server not found" | `.mcp.json` 바이너리 경로 확인; `cq doctor` 실행 |
| macOS 코드 서명 에러 | `go build -o` 직접 사용, `cp` 금지 |
| Python sidecar 에러 | `uv sync` 실행; Python 3.11+ 확인 |
