# SupervisorBackend 확장 가이드

이 문서는 Supervisor 백엔드의 구조와 커스텀 백엔드 구현 방법을 설명합니다.

## 개요

C4는 플러그인 가능한 Supervisor 아키텍처를 제공합니다:

```
┌─────────────────────────────────────────────────────┐
│                    Supervisor                        │
│                        │                             │
│                        v                             │
│    ┌────────────────────────────────────┐           │
│    │       SupervisorBackend (ABC)       │          │
│    └────────────────────────────────────┘           │
│                        │                             │
│         ┌──────────────┼──────────────┐             │
│         v              v              v             │
│    ClaudeCLI      LiteLLM         Mock             │
│    (기본값)      (100+ LLM)     (테스트용)          │
└─────────────────────────────────────────────────────┘
```

## 내장 백엔드

### 1. ClaudeCliBackend (기본값)

Claude Code CLI를 사용하는 기본 구현:

```python
from c4.supervisor import ClaudeCliBackend

backend = ClaudeCliBackend(
    working_dir=Path("."),
    max_retries=3,
    model=None,  # CLI 기본값 사용
)
```

**특징:**
- Claude Code 구독 사용
- 별도 API 키 불필요
- `claude -p` CLI 호출

### 2. LiteLLMBackend (Multi-Provider)

LiteLLM을 통해 100+ LLM Provider 지원:

```python
from c4.supervisor import LiteLLMBackend

backend = LiteLLMBackend(
    model="gpt-4o",
    api_key="sk-...",
    max_retries=3,
    timeout=300,
    temperature=0.0,
    max_tokens=4096,
)
```

**지원 Provider:**
- OpenAI (gpt-4o, o1)
- Anthropic (claude-3-opus)
- Azure OpenAI
- Ollama (로컬)
- Bedrock, Groq, Together, ZhipuAI 등

### 3. MockBackend (테스트용)

테스트를 위한 Mock 구현:

```python
from c4.supervisor import MockBackend
from c4.models import SupervisorDecision

backend = MockBackend(
    decision=SupervisorDecision.APPROVE,
    notes="Auto-approved for testing",
)
```

## Backend Factory

설정 기반 백엔드 생성:

```python
from c4.supervisor import create_backend
from c4.models import LLMConfig

# OpenAI 백엔드 생성
config = LLMConfig(model="gpt-4o", api_key_env="OPENAI_API_KEY")
backend = create_backend(config)

# Claude CLI 백엔드 (기본값)
config = LLMConfig()  # model="claude-cli"
backend = create_backend(config)
```

설정 파일에서 자동 로드:

```python
from c4.supervisor import create_backend_from_config_file
from pathlib import Path

# .c4/config.yaml에서 llm 섹션 읽어서 백엔드 생성
backend = create_backend_from_config_file(
    c4_dir=Path(".c4"),
    working_dir=Path(".")
)
```

## Protocol 정의

### SupervisorResponse

```python
from dataclasses import dataclass
from c4.models import SupervisorDecision

@dataclass
class SupervisorResponse:
    """Supervisor 리뷰 결과"""
    decision: SupervisorDecision  # APPROVE, REQUEST_CHANGES, REPLAN
    checkpoint_id: str            # 리뷰한 체크포인트 ID
    notes: str                    # 리뷰 코멘트
    required_changes: list[str]   # 요청된 변경사항

    @classmethod
    def from_dict(cls, data: dict) -> "SupervisorResponse":
        """딕셔너리에서 생성"""
        ...
```

### SupervisorBackend (ABC)

```python
from abc import ABC, abstractmethod
from pathlib import Path

class SupervisorBackend(ABC):
    """Supervisor 백엔드 추상 클래스"""

    @abstractmethod
    def run_review(
        self,
        prompt: str,
        bundle_dir: Path,
        timeout: int = 300,
    ) -> SupervisorResponse:
        """Supervisor 리뷰를 실행합니다."""
        pass

    @property
    @abstractmethod
    def name(self) -> str:
        """백엔드 이름 (로깅용)"""
        pass

    def save_response(
        self, bundle_dir: Path, response: SupervisorResponse
    ) -> None:
        """응답을 번들 디렉토리에 저장 (기본 구현)"""
        ...
```

## 커스텀 Backend 구현

### 예시: Human Review Backend

```python
from pathlib import Path
from c4.supervisor.backend import (
    SupervisorBackend,
    SupervisorResponse,
    SupervisorError,
)
from c4.models import SupervisorDecision

class HumanReviewBackend(SupervisorBackend):
    """사람이 직접 리뷰하는 백엔드"""

    @property
    def name(self) -> str:
        return "human"

    def run_review(
        self,
        prompt: str,
        bundle_dir: Path,
        timeout: int = 300,
    ) -> SupervisorResponse:
        # 프롬프트 출력
        print("=" * 60)
        print("SUPERVISOR REVIEW REQUIRED")
        print("=" * 60)
        print(prompt)
        print("=" * 60)

        # 사용자 입력
        print("\nDecision (APPROVE/REQUEST_CHANGES/REPLAN):")
        decision_str = input("> ").strip().upper()

        print("\nNotes:")
        notes = input("> ").strip()

        changes = []
        if decision_str == "REQUEST_CHANGES":
            print("\nRequired changes (empty to finish):")
            while True:
                change = input("> ").strip()
                if not change:
                    break
                changes.append(change)

        try:
            decision = SupervisorDecision(decision_str)
        except ValueError:
            raise SupervisorError(f"Invalid decision: {decision_str}")

        response = SupervisorResponse(
            decision=decision,
            checkpoint_id="",
            notes=notes,
            required_changes=changes,
        )

        self.save_response(bundle_dir, response)
        return response
```

### 예시: Webhook Backend

```python
import requests
from pathlib import Path
from c4.supervisor.backend import (
    SupervisorBackend,
    SupervisorResponse,
    SupervisorError,
)

class WebhookBackend(SupervisorBackend):
    """외부 서비스로 리뷰 요청을 보내는 백엔드"""

    def __init__(self, webhook_url: str, api_key: str):
        self.webhook_url = webhook_url
        self.api_key = api_key

    @property
    def name(self) -> str:
        return "webhook"

    def run_review(
        self,
        prompt: str,
        bundle_dir: Path,
        timeout: int = 300,
    ) -> SupervisorResponse:
        (bundle_dir / "prompt.md").write_text(prompt)

        try:
            response = requests.post(
                self.webhook_url,
                headers={"Authorization": f"Bearer {self.api_key}"},
                json={"prompt": prompt},
                timeout=timeout,
            )
            response.raise_for_status()
            data = response.json()

            result = SupervisorResponse.from_dict(data)
            self.save_response(bundle_dir, result)
            return result

        except Exception as e:
            raise SupervisorError(f"Webhook failed: {e}")
```

## ResponseParser

공통 JSON 파싱 로직:

```python
from c4.supervisor import ResponseParser

# LLM 출력에서 응답 파싱
output = '''
```json
{"decision": "APPROVE", "checkpoint": "CP-001", "notes": "LGTM"}
```
'''

response = ResponseParser.parse(output)
# SupervisorResponse(decision=APPROVE, ...)
```

**파싱 전략:**
1. ```json 코드 블록에서 JSON 추출
2. `"decision"` 키가 있는 raw JSON 찾기
3. 전체 출력을 JSON으로 파싱

## Supervisor 사용

### 백엔드 직접 주입

```python
from c4.supervisor import Supervisor, LiteLLMBackend

backend = LiteLLMBackend(model="gpt-4o", api_key="sk-...")
supervisor = Supervisor(
    project_root=Path("."),
    backend=backend,
)

response = supervisor.run_supervisor(bundle_dir)
```

### LLMConfig로 주입

```python
from c4.supervisor import Supervisor
from c4.models import LLMConfig

config = LLMConfig(model="gpt-4o", api_key_env="OPENAI_API_KEY")
supervisor = Supervisor(
    project_root=Path("."),
    llm_config=config,
)
```

### 설정 파일 자동 로드

```python
from c4.supervisor import Supervisor

# .c4/config.yaml의 llm 섹션 자동 로드
supervisor = Supervisor(project_root=Path("."))
```

**우선순위:**
1. `backend` 파라미터 (명시적)
2. `llm_config` 파라미터
3. `.c4/config.yaml`의 llm 섹션
4. 기본값: ClaudeCliBackend

## 응답 형식

Supervisor는 다음 JSON 형식으로 응답해야 합니다:

```json
{
  "decision": "APPROVE",
  "checkpoint": "CP-001",
  "notes": "코드가 잘 구현되었습니다.",
  "required_changes": []
}
```

**REQUEST_CHANGES 예시:**
```json
{
  "decision": "REQUEST_CHANGES",
  "checkpoint": "CP-001",
  "notes": "수정이 필요합니다.",
  "required_changes": [
    "테스트 커버리지 80% 이상으로 증가",
    "에러 핸들링 추가"
  ]
}
```

## 테스트

```python
import pytest
from pathlib import Path
from c4.models import SupervisorDecision

@pytest.fixture
def backend():
    return MyCustomBackend()

class TestMyBackend:
    def test_name(self, backend):
        assert backend.name == "my-backend"

    def test_review_approve(self, backend, tmp_path):
        bundle_dir = tmp_path / "bundle"
        bundle_dir.mkdir()

        response = backend.run_review(
            prompt="Review this code",
            bundle_dir=bundle_dir,
            timeout=30,
        )

        assert response.decision in SupervisorDecision
        assert (bundle_dir / "response.json").exists()
```

## 고려사항

### 1. 에러 처리
- 네트워크 오류, 타임아웃 처리
- 재시도 로직 구현
- `SupervisorError` 발생

### 2. 비용 관리
- API 호출 비용 모니터링
- LiteLLMBackend의 `last_usage` 속성 활용
- 캐싱 전략 고려

### 3. 보안
- API 키는 환경 변수 사용
- 프롬프트 인젝션 방지
- 민감 정보 로깅 주의

## 다음 단계

- [LLM 설정](../user-guide/LLM-설정.md) - Provider별 설정 방법
- [StateStore 확장](StateStore-확장.md) - 커스텀 저장소 구현
- [아키텍처](아키텍처.md) - 전체 시스템 구조
