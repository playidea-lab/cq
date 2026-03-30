# 티어

CQ에는 세 가지 티어가 있습니다. **Free**로 시작하고, 어디서든 GPU 접근과 클라우드 동기화를 원할 때 **Pro**로 업그레이드하고, 팀 GPU 워크로드가 필요하면 **Team**을 추가하세요.

## 비교

| 기능 | Free | Pro | Team |
|------|------|-----|------|
| 태스크 오케스트레이션 | 로컬 SQLite | Supabase (클라우드) | Supabase (클라우드) |
| 지식 베이스 | 로컬 SQLite | pgvector (클라우드) | pgvector (클라우드) |
| 멀티 Worker 실행 | 단일 머신 | 모든 머신 | 모든 머신 |
| Knowledge Loop | 선호도만 | 전체 (크로스 세션) | 전체 (크로스 세션) |
| Research Loop | 없음 | 있음 | 있음 |
| Remote AI 워크스페이스 (ChatGPT/Claude Desktop) | 없음 | 있음 | 있음 |
| Drive (파일 스토리지) | 없음 | 있음 | 있음 |
| Hub (분산 GPU 작업) | 없음 | 있음 | 있음 |
| Relay (NAT 통과, 종단간 암호화) | 없음 | 있음 | 있음 |
| 연결 가능 Worker 수 | 1개 (로컬만) | 무제한 | 무제한 |
| 필요한 API 키 | 있음 (직접 관리) | 0개 | 0개 |
| 가격 | 무료 | $5–10/월 | 문의 |
| 설정 | `config.yaml` 필요 | `cq auth login` | `cq auth login` + `cq serve` |

## Free

모든 것이 로컬에서 실행됩니다. LLM API 키를 직접 관리합니다.

```yaml
# .c4/config.yaml
llm_gateway:
  enabled: true
  default: openai
  providers:
    openai:
      enabled: true
      default_model: gpt-4o-mini
```

```sh
cq secret set openai.api_key    # ~/.c4/secrets.db에 암호화되어 저장
```

적합한 경우: 오프라인 사용, 에어갭 환경, 완전한 데이터 제어.

## Pro

어디서나 GPU 연결. API 키 불필요 — CQ의 LLM 프록시가 처리합니다.

```sh
cq auth login    # GitHub OAuth, 최초 1회
cq serve         # relay + 이벤트 동기화 + 토큰 갱신 시작
cq claude        # 개발 시작
```

제공하는 것:
- 종단간 암호화 relay로 GPU Worker 연결 — 어떤 머신, 어떤 네트워크
- 세션과 AI 도구를 넘어 지식이 지속됩니다
- [Knowledge Loop](growth-loop.md)가 선호도와 실험 결과를 자동으로 쌓습니다
- [Remote AI 워크스페이스](remote-brain.md) — ChatGPT, Claude Desktop, Cursor에서 지식 접근
- `cq relay call` — NAT를 통해 다른 CQ 인스턴스에 도달

적합한 경우: 어디서나 GPU 접근과 모든 도구에서 영구적인 AI 메모리를 원하는 개인 개발자 및 ML 연구자.

## Team

**Pro**의 모든 기능에 팀 GPU 인프라가 추가됩니다.

```sh
cq auth login
cq serve
```

GPU 머신(또는 클라우드 VM)에서:

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq auth login
cq serve    # 이 머신이 GPU Worker가 됨
```

추가로 제공하는 것 (Pro 기반):
- 노트북에서 ML 훈련 작업을 제출하고 공유 GPU 서버에서 실행
- **Research Loop** — 자율 실험 사이클: 계획 → 훈련 → 평가 → 반복
- Drive — TUS 재개 가능 업로드와 내용 주소 지정 버전 관리가 있는 클라우드 파일 스토리지
- 아티팩트 업로드 — 작업 완료 시 결과물이 자동으로 Drive에 저장
- DAG 엔진 — 자동 의존성 해결로 종속 작업 체이닝
- 팀 지식 풀 — 공유된 실험 결과와 모범 사례

적합한 경우: ML 연구팀, 원격 GPU 인프라를 가진 조직.

## Knowledge Loop (모든 티어)

Knowledge Loop는 모든 티어에서 사용 가능하지만, **Pro**와 **Team**에서 가장 잘 작동합니다:

```
세션 1: 실수를 수정함            → 선호도 저장 (count: 1)
세션 3: 같은 선호도 다시 감지됨  → CLAUDE.md에 힌트 추가
세션 5: 5번째                   → 영구 규칙으로 승격
세션 6+: AI가 프롬프트 없이 규칙을 따름
```

자세한 내용은 [Knowledge Loop](growth-loop.md)를 참고하세요.

## Research Loop (Pro 및 Team)

Research Loop가 연결된 GPU Worker에서 자율적으로 실행됩니다:

```
실험 계획 → Hub에 제출 → GPU에서 훈련 → 메트릭 평가
    ↑                                           │
    └───────────────────────────────────────────┘
            (종료 조건까지 반복)
```

루프 시작:

```sh
cq research run --goal "H36M에서 MPJPE 최대화" --budget 10
```

결과는 Hub를 통해 실시간으로 스트리밍됩니다. 최적 체크포인트는 자동으로 Drive에 저장됩니다.

## 티어 전환

티어는 로그인 상태와 `cq serve` 실행 여부로 결정됩니다. 별도의 설정 플래그가 없습니다.

- **Free**: 로그인하지 않거나 config에 `cloud.url` 없음
- **Pro**: 로그인 + `cq serve` 실행 (Team Hub 미설정)
- **Team**: 로그인 + `cq serve` + 여러 GPU Worker 연결

SQLite(Free)에 저장된 데이터는 업그레이드 시 보존됩니다. 클라우드 동기화가 로컬에서 중단된 지점부터 이어집니다.
