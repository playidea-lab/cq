# 튜토리얼: CQ 첫 5분

> AI 코딩 도구가 처음인 연구원을 위한 가이드.
> ML/DL 지식 불필요 — 터미널과 프로젝트만 있으면 됩니다.

## 이 튜토리얼에서 할 것

1. CQ 설치 (30초)
2. 프로젝트에서 시작 (10초)
3. 뭔가 시켜보기 (첫 태스크)
4. 결과 확인

## 1단계: 설치

터미널을 열고 붙여넣기:

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

터미널을 닫고 다시 열거나 `source ~/.zshrc` 실행.

## 2단계: 프로젝트에서 CQ 시작

```sh
cd ~/my-project    # 코드가 있는 아무 프로젝트
cq
```

이렇게 나오면 성공:

```
CQ ready.
```

끝입니다. CQ가 알아서:
- Claude Code 연결
- 프로젝트 지식 베이스 설정
- 백그라운드 서비스 시작
- `CLAUDE.md` 생성 (프로젝트 AI 지침)

## 3단계: 뭔가 시켜보기

이제 Claude Code + CQ 상태입니다. 원하는 걸 말하세요:

### 작은 요청 (직접 수정)

```
README.md에 프로젝트 설명 추가해줘
```

CQ가 작은 작업으로 판단하고 직접 수정합니다. 오버헤드 없음.

### 중간 요청 (워커 1개)

```
이 코드 리뷰해줘
```

6축 코드 리뷰 실행: 정확성, 보안, 신뢰성, 관측성, 테스트 커버리지, 가독성.

### 큰 요청 (전체 파이프라인)

```
로그인 기능 만들어줘
```

CQ가 전체 파이프라인을 자동 실행:

```
/pi (브레인스토밍) → /c4-plan (설계) → /c4-run (구현) → /c4-finish (검증)
```

여러 워커가 병렬로 태스크를 수행합니다. 각자 독립된 git 브랜치. 모든 구현은 리뷰 후 병합.

## 4단계: 결과 확인

```
/c4-status
```

태스크 상태, 진행률, 다음 할 것을 보여줍니다.

## CQ가 뒤에서 하는 것

| 보이는 것 | CQ가 하는 것 |
|----------|------------|
| "CQ ready." | 프로젝트 초기화, MCP 서버 연결, 백그라운드 서비스 시작 |
| 요청 입력 | Small/Medium/Large 자동 라우팅 |
| 워커 실행 | 각 워커: 독립 git worktree, 태스크 1개, 새 컨텍스트 |
| 코드 제출 | 6축 리뷰 자동 실행 |
| 세션 종료 | 지식 기록, 다음 세션을 위한 패턴 학습 |

## 주요 명령

| 명령 | 기능 |
|------|------|
| `/pi` | 아이디어 탐색·토론 |
| `/c4-plan` | 기능 계획 + 태스크 분해 |
| `/c4-run` | 워커 병렬 실행 |
| `/c4-status` | 진행 상황 확인 |
| `/c4-quick` | 빠른 단일 태스크 |

## 팀에서 쓸 때

팀원이 같은 repo를 클론하면 `.mcp.json`과 `CLAUDE.md`가 이미 있습니다. 그냥:

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq
```

CQ의 지식 베이스는 공유됩니다 — 한 사람이 배운 것을 모두가 활용.

## 문제 해결

| 문제 | 해결 |
|------|------|
| `cq: command not found` | 터미널 재시작 또는 `source ~/.zshrc` |
| MCP 도구 안 뜸 | `cq doctor --fix` 실행 |
| 워커 연결 안 됨 | `cq serve status` 확인 |
| 도움 필요 | [GitHub Discussions에서 질문](https://github.com/PlayIdea-Lab/cq/discussions) |

## 다음

- [전체 워크플로우 →](/ko/workflow/)
- [원격 GPU 워커 설정 →](/ko/guide/worker-setup)
- [이슈 보고 →](https://github.com/PlayIdea-Lab/cq/issues)
