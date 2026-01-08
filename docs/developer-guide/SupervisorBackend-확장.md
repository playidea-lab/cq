# SupervisorBackend 확장 가이드

이 문서는 커스텀 Supervisor 백엔드를 구현하는 방법을 설명합니다.

## 개요

C4는 플러그인 가능한 Supervisor 아키텍처를 제공합니다:

```
┌─────────────────────────────────────┐
│           Supervisor                │
│              │                      │
│              v                      │
│    ┌─────────────────────┐          │
│    │ SupervisorBackend   │ ◄─────── Protocol (ABC)
│    │     (Protocol)      │          │
│    └─────────────────────┘          │
│              │                      │
│     ┌────────┼────────┐             │
│     v        v        v             │
│ ClaudeCLI  OpenAI   Mock            │
└─────────────────────────────────────┘
```

## Protocol 정의

### SupervisorResponse

```python
# c4/supervisor/backend.py

from dataclasses import dataclass
from c4.models import SupervisorDecision

@dataclass
class SupervisorResponse:
    """Supervisor 리뷰 결과"""

    decision: SupervisorDecision  # APPROVE, REQUEST_CHANGES, REPLAN
    checkpoint_id: str            # 리뷰한 체크포인트 ID
    notes: str                    # 리뷰 코멘트
    required_changes: list[str]   # 요청된 변경사항 (REQUEST_CHANGES 시)

    @classmethod
    def from_dict(cls, data: dict) -> "SupervisorResponse":
        """딕셔너리에서 생성"""
        decision = SupervisorDecision(data["decision"].upper())
        return cls(
            decision=decision,
            checkpoint_id=data.get("checkpoint", ""),
            notes=data.get("notes", ""),
            required_changes=data.get("required_changes", []),
        )
```

### SupervisorBackend

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
        """
        Supervisor 리뷰를 실행합니다.

        Args:
            prompt: 렌더링된 리뷰 프롬프트
            bundle_dir: 번들 디렉토리 (아티팩트 저장용)
            timeout: 타임아웃 (초)

        Returns:
            SupervisorResponse (결정, 노트, 변경사항)

        Raises:
            SupervisorError: 리뷰 실패 시
        """
        pass

    @property
    @abstractmethod
    def name(self) -> str:
        """백엔드 이름 (로깅/디버깅용)"""
        pass

    def save_response(
        self, bundle_dir: Path, response: SupervisorResponse
    ) -> None:
        """응답을 번들 디렉토리에 저장 (기본 구현)"""
        import json
        (bundle_dir / "response.json").write_text(
            json.dumps(response.to_dict(), indent=2)
        )
```

## 기존 구현 예시

### ClaudeCliBackend

Claude CLI를 사용하는 기본 구현:

```python
# c4/supervisor/claude_backend.py

class ClaudeCliBackend(SupervisorBackend):
    def __init__(
        self,
        working_dir: Path | None = None,
        max_retries: int = 3,
        model: str | None = None,
    ):
        self.working_dir = working_dir or Path.cwd()
        self.max_retries = max_retries
        self.model = model

    @property
    def name(self) -> str:
        return "claude-cli"

    def run_review(
        self,
        prompt: str,
        bundle_dir: Path,
        timeout: int = 300,
    ) -> SupervisorResponse:
        # 프롬프트 저장
        (bundle_dir / "prompt.md").write_text(prompt)

        # Claude CLI 실행
        cmd = ["claude", "-p", prompt]
        if self.model:
            cmd.extend(["--model", self.model])

        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=timeout,
            cwd=self.working_dir,
        )

        # 응답 파싱
        response = self._parse_decision(result.stdout)
        self.save_response(bundle_dir, response)

        return response
```

### MockBackend

테스트용 Mock 구현:

```python
# c4/supervisor/mock_backend.py

class MockBackend(SupervisorBackend):
    def __init__(
        self,
        decision: SupervisorDecision = SupervisorDecision.APPROVE,
        notes: str = "Auto-approved by mock",
    ):
        self._decision = decision
        self._notes = notes

    @property
    def name(self) -> str:
        return "mock"

    def run_review(
        self,
        prompt: str,
        bundle_dir: Path,
        timeout: int = 300,
    ) -> SupervisorResponse:
        response = SupervisorResponse(
            decision=self._decision,
            checkpoint_id="",  # Supervisor가 설정
            notes=self._notes,
            required_changes=[],
        )
        self.save_response(bundle_dir, response)
        return response
```

## 커스텀 Backend 구현하기

### 예시: OpenAI Backend

```python
# my_project/openai_backend.py

import json
from pathlib import Path
from openai import OpenAI
from c4.supervisor.backend import (
    SupervisorBackend,
    SupervisorResponse,
    SupervisorError,
)

class OpenAIBackend(SupervisorBackend):
    """OpenAI GPT-4를 사용하는 Supervisor 백엔드"""

    def __init__(
        self,
        api_key: str | None = None,
        model: str = "gpt-4-turbo-preview",
    ):
        self.client = OpenAI(api_key=api_key)
        self.model = model

    @property
    def name(self) -> str:
        return f"openai-{self.model}"

    def run_review(
        self,
        prompt: str,
        bundle_dir: Path,
        timeout: int = 300,
    ) -> SupervisorResponse:
        # 프롬프트 저장
        (bundle_dir / "prompt.md").write_text(prompt)

        try:
            # OpenAI API 호출
            response = self.client.chat.completions.create(
                model=self.model,
                messages=[
                    {
                        "role": "system",
                        "content": (
                            "You are a code reviewer. "
                            "Respond with a JSON object containing: "
                            "decision (APPROVE/REQUEST_CHANGES/REPLAN), "
                            "notes (your review comments), "
                            "required_changes (list of changes if any)."
                        ),
                    },
                    {"role": "user", "content": prompt},
                ],
                response_format={"type": "json_object"},
                timeout=timeout,
            )

            # 응답 파싱
            content = response.choices[0].message.content
            data = json.loads(content)
            result = SupervisorResponse.from_dict(data)

            # 응답 저장
            self.save_response(bundle_dir, result)

            return result

        except Exception as e:
            raise SupervisorError(f"OpenAI review failed: {e}")
```

### 예시: GitHub Copilot Backend

```python
# my_project/copilot_backend.py

import subprocess
from pathlib import Path
from c4.supervisor.backend import (
    SupervisorBackend,
    SupervisorResponse,
    SupervisorError,
)

class CopilotBackend(SupervisorBackend):
    """GitHub Copilot CLI를 사용하는 Supervisor 백엔드"""

    @property
    def name(self) -> str:
        return "copilot"

    def run_review(
        self,
        prompt: str,
        bundle_dir: Path,
        timeout: int = 300,
    ) -> SupervisorResponse:
        # 프롬프트 저장
        (bundle_dir / "prompt.md").write_text(prompt)

        # Copilot CLI 실행 (예시)
        result = subprocess.run(
            ["gh", "copilot", "suggest", "-t", "code"],
            input=prompt,
            capture_output=True,
            text=True,
            timeout=timeout,
        )

        # 응답 파싱 (Copilot 출력 형식에 맞게 구현)
        response = self._parse_response(result.stdout)
        self.save_response(bundle_dir, response)

        return response

    def _parse_response(self, output: str) -> SupervisorResponse:
        # Copilot 출력을 파싱하여 SupervisorResponse 생성
        # 실제 구현은 Copilot 출력 형식에 따라 다름
        ...
```

### 예시: Human Review Backend

```python
# my_project/human_backend.py

from pathlib import Path
from c4.supervisor.backend import (
    SupervisorBackend,
    SupervisorResponse,
    SupervisorError,
)
from c4.models import SupervisorDecision

class HumanReviewBackend(SupervisorBackend):
    """사람이 직접 리뷰하는 백엔드 (CLI 입력)"""

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
            print("\nRequired changes (one per line, empty to finish):")
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

## Backend 사용하기

### Supervisor에 주입

```python
from c4.supervisor import Supervisor
from my_project.openai_backend import OpenAIBackend

# 커스텀 백엔드 생성
backend = OpenAIBackend(model="gpt-4")

# Supervisor에 주입
supervisor = Supervisor(
    state_machine=daemon.state_machine,
    backend=backend,
)
```

### 설정 파일로 전환

`.c4/config.yaml`:
```yaml
supervisor:
  backend: openai
  openai:
    model: gpt-4-turbo-preview
    # api_key는 환경 변수에서
```

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

REQUEST_CHANGES 예시:
```json
{
  "decision": "REQUEST_CHANGES",
  "checkpoint": "CP-001",
  "notes": "몇 가지 수정이 필요합니다.",
  "required_changes": [
    "테스트 커버리지 80% 이상으로 증가",
    "에러 핸들링 추가"
  ]
}
```

## 테스트

### Backend 테스트 패턴

```python
# tests/test_my_backend.py

import pytest
from pathlib import Path
from my_project.openai_backend import OpenAIBackend
from c4.models import SupervisorDecision

@pytest.fixture
def backend():
    return OpenAIBackend()

@pytest.fixture
def bundle_dir(tmp_path):
    return tmp_path / "bundle"

class TestOpenAIBackend:
    def test_name(self, backend):
        assert "openai" in backend.name

    def test_review_approve(self, backend, bundle_dir):
        bundle_dir.mkdir()

        response = backend.run_review(
            prompt="Review this: LGTM",
            bundle_dir=bundle_dir,
            timeout=30,
        )

        assert response.decision in SupervisorDecision
        assert (bundle_dir / "response.json").exists()
```

## 고려사항

### 1. 에러 처리
- 네트워크 오류, 타임아웃 처리
- 재시도 로직 (ClaudeCliBackend 참조)
- `SupervisorError` 발생

### 2. 응답 파싱
- JSON 형식 강제 (response_format 사용)
- 다양한 출력 형식 처리
- 실패 시 명확한 에러 메시지

### 3. 비용 관리
- API 호출 비용 고려
- 캐싱 전략
- 토큰 사용량 모니터링

### 4. 보안
- API 키 환경 변수 사용
- 프롬프트 인젝션 방지
- 민감 정보 로깅 주의

## 다음 단계

- [StateStore 확장](StateStore-확장.md) - 커스텀 저장소 구현
- [아키텍처](아키텍처.md) - 전체 시스템 구조
