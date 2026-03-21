# CQ — AI 프로젝트 오케스트레이션 엔진

**한국어** | [English](README.md)

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

**CQ**는 로컬 퍼스트 AI 오케스트레이션 플랫폼입니다.
계획 수립부터 구현, 리뷰, 배포까지 전체 개발 라이프사이클을 자동화하며, Supabase를 통해 여러 머신에 작업을 분산합니다.

## 구조

```
내 노트북                              원격 Worker (GPU 서버 등)
┌──────────────────────┐             ┌──────────────────────┐
│  cq                  │             │  cq hub worker start │
│  ├── Claude Code     │   Supabase  │  ├── LISTEN/NOTIFY   │
│  ├── 133개 MCP 도구  │◄──────────►│  ├── 잡 수령          │
│  ├── 텔레그램 봇     │  (잡, 지식) │  ├── 실행            │
│  └── 지식 DB         │             │  └── 결과 보고       │
└──────────────────────┘             └──────────────────────┘
```

- **서버 불필요** — Supabase가 잡 큐, 지식 동기화, 인증, 스토리지 처리
- **모든 AI CLI 지원** — Claude Code, Gemini CLI, Codex, Cursor
- **텔레그램 제어** — 봇 페어링 후 폰에서 컨트롤

## 빠른 시작

```sh
# 설치
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh

# 로그인
cq auth login

# Claude Code 시작
cq claude

# (선택) 텔레그램 봇 페어링
cq setup
```

## 분산 워커

여러 머신에 실험을 분산 — 관리할 서버 없음:

```sh
# GPU 서버에서:
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq auth login --device
cq hub worker start     # Supabase 연결, 잡 대기

# 내 노트북에서:
cq hub submit --run "python train.py --lr 1e-4"
# → Worker가 수령해서 실행
```

## 워크플로우

```
/pi "아이디어..."                    → 아이디어 탐색 → idea.md
/c4-plan "기능 설명"                 → 요구사항 분석 + 설계 + 태스크
/c4-run                             → 워커 스폰, 병렬 구현
/c4-finish                          → 빌드 · 테스트 · 문서 · 커밋
/c4-status                          → 진행 상황 확인
```

## 사용 예시

### 기능 구현

> **당신:** "JWT 인증 추가해줘"

```
/c4-plan "JWT auth with Google and GitHub OAuth"

  ● Discovery → Design → Tasks
    T-001  OAuth 프로바이더 인터페이스
    T-002  Google 프로바이더
    T-003  GitHub 프로바이더
    T-004  JWT 미들웨어
    T-005  통합 테스트

/c4-run
  → 워커들이 병렬로 구현 → 리뷰 → 완료
```

### 분산 실험

> **당신:** "backbone 3개 비교 돌려"

```
cq hub submit --run "python train.py --backbone resnet50"
cq hub submit --run "python train.py --backbone efficientnet"
cq hub submit --run "python train.py --backbone vit"

# 여러 머신의 Worker가 병렬로 수령, 실행
# 결과는 Supabase 지식 베이스에 축적
```

## 텔레그램

텔레그램 봇을 페어링하면 폰에서 CQ를 제어할 수 있습니다:

```sh
cq setup    # BotFather → 토큰 → 페어링 (1회)
cq          # 봇 선택 → Claude Code + 텔레그램 세션
```

텔레그램에서:
- 메시지 전송 → Claude Code가 처리하고 응답
- 태스크/실험 완료 시 알림 수신
- 메모장으로 사용 — 노트북 꺼져있어도 메시지 큐잉

## 지식 시스템

CQ는 모든 태스크와 실험에서 지식을 축적합니다:

```
태스크 완료 → 자동 기록 (발견, 패턴, 우려사항)
                  ↓
            지식 DB (FTS + 벡터 검색)
                  ↓
다음 태스크 할당 → 관련 지식 자동 주입
                  ↓
            더 나은 구현 → 기록 → ...
```

자기강화 루프. Supabase를 통해 디바이스 간 동기화.

## Soul & 학습

CQ는 시간이 지나면서 코딩 스타일을 학습합니다:

- **Persona** — git diff에서 패턴 추출
- **POP** — Personal Ontology Pipeline (대화 → 지식)
- **Soul** — 리뷰 우선순위, 품질 철학

## 세션

```sh
cq claude -t myproject    # 세션 시작 또는 재개
cq ls                     # 봇 및 세션 목록
```

## 설정

```sh
cq auth login              # GitHub OAuth → 클라우드 자동 설정
cq auth login --device     # 헤드리스/SSH: 디바이스 코드 플로우
cq doctor                  # 설치 상태 확인
```

수동 설정 편집 불필요. `cq auth login`이 모든 것을 설정합니다.

## 업데이트

설치 명령을 다시 실행하면 됩니다:

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## 요구사항

- macOS Apple Silicon (arm64) 또는 Linux (amd64 / arm64)
- [Claude Code](https://claude.ai/code) 설치됨
- `curl` 사용 가능

## 라이선스

[MIT + Commons Clause](LICENSE) — 자유롭게 사용 및 수정 가능, 상업적 재판매 금지.
