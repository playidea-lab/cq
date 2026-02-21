# CQ - AI 코딩 에이전트 관리 엔진

CQ는 Claude Code와 함께 사용하는 프로젝트 관리 엔진입니다.
C4 Engine을 통해 계획부터 완료까지 자동화된 워크플로우를 제공합니다.

## 설치 (curl)

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## 티어 선택

| 티어 | 설명 |
|------|------|
| solo | 로컬 전용 (기본값) |
| connected | Supabase + C3 EventBus 연동 |
| full | 모든 기능 (Hub, Drive, LLM Gateway 포함) |

```sh
# connected 티어 설치
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier connected

# full 티어 설치
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier full
```

## 빠른 시작

```sh
# 설치 확인
cq doctor

# 프로젝트 초기화
cq init

# 상태 확인
cq status
```

## 설정 템플릿

`configs/` 디렉토리에서 각 티어별 기본 설정 템플릿을 확인할 수 있습니다.
`configs/solo.yaml`, `configs/connected.yaml`, `configs/full.yaml`을 참고하세요.

## 아티팩트 이름 규약

빌드 아티팩트는 `cq-{tier}-{GOOS}-{GOARCH}` 형식을 따릅니다.
예: `cq-solo-linux-amd64`, `cq-connected-darwin-arm64`, `cq-full-linux-arm64`

## 라이선스

MIT
