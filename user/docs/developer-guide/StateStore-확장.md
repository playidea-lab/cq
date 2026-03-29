# StateStore 확장 가이드

이 문서는 커스텀 StateStore 백엔드를 구현하는 방법을 설명합니다.

## 개요

C4는 플러그인 가능한 저장소 아키텍처를 제공합니다:

```
┌─────────────────────────────────┐
│         StateMachine           │
│              │                 │
│              v                 │
│    ┌─────────────────┐         │
│    │   StateStore    │ ◄─────── Protocol (ABC)
│    │    (Protocol)   │         │
│    └─────────────────┘         │
│              │                 │
│     ┌────────┼────────┐        │
│     v        v        v        │
│  LocalFile  SQLite  Supabase   │
└─────────────────────────────────┘
```

## Protocol 정의

### StateStore

```python
# c4/store/protocol.py

from abc import ABC, abstractmethod
from c4.models import C4State

class StateStore(ABC):
    """상태 저장소 추상 클래스"""

    @abstractmethod
    def load(self, project_id: str) -> C4State:
        """
        프로젝트 상태를 로드합니다.

        Args:
            project_id: 프로젝트 식별자

        Returns:
            C4State 인스턴스

        Raises:
            StateNotFoundError: 상태가 존재하지 않을 때
        """
        pass

    @abstractmethod
    def save(self, state: C4State) -> None:
        """
        프로젝트 상태를 저장합니다.

        Args:
            state: 저장할 상태 (project_id는 state에 포함)

        Note:
            save() 호출 시 state.updated_at을 자동으로 업데이트해야 합니다.
        """
        pass

    @abstractmethod
    def exists(self, project_id: str) -> bool:
        """프로젝트 상태가 존재하는지 확인합니다."""
        pass

    @abstractmethod
    def delete(self, project_id: str) -> None:
        """프로젝트 상태를 삭제합니다."""
        pass

    @abstractmethod
    @contextmanager
    def atomic_modify(
        self, project_id: str
    ) -> Generator[C4State, None, None]:
        """
        원자적으로 상태를 로드, 수정, 저장합니다.

        동시성 제어를 위한 필수 메서드:
        - SQLite: WAL 모드 + 파일 락
        - Supabase: Optimistic locking (version 기반)
        - LocalFile: fcntl 파일 락

        Usage:
            with store.atomic_modify(project_id) as state:
                state.queue.done.append(task_id)
                del state.queue.in_progress[task_id]
            # 컨텍스트 종료 시 자동 저장

        Raises:
            StateNotFoundError: 상태가 존재하지 않을 때
            ConcurrentModificationError: 동시 수정 충돌
        """
        pass
```

### LockStore

```python
class LockStore(ABC):
    """분산 잠금 저장소 추상 클래스"""

    @abstractmethod
    def acquire_scope_lock(
        self,
        project_id: str,
        scope: str,
        owner: str,
        ttl_seconds: int,
    ) -> bool:
        """
        Scope 잠금을 획득합니다.

        Args:
            project_id: 프로젝트 ID
            scope: 잠금 범위 (예: "src/backend")
            owner: 잠금 소유자 (예: worker_id)
            ttl_seconds: 잠금 유효 시간

        Returns:
            True: 획득 성공
            False: 다른 소유자가 보유 중
        """
        pass

    @abstractmethod
    def release_scope_lock(self, project_id: str, scope: str) -> bool:
        """잠금을 해제합니다."""
        pass

    @abstractmethod
    def refresh_scope_lock(
        self,
        project_id: str,
        scope: str,
        owner: str,
        ttl_seconds: int,
    ) -> bool:
        """잠금 TTL을 갱신합니다. 소유자가 일치해야 합니다."""
        pass

    @abstractmethod
    def get_scope_lock(
        self,
        project_id: str,
        scope: str,
    ) -> tuple[str, datetime] | None:
        """현재 잠금 소유자와 만료 시간을 반환합니다."""
        pass

    @abstractmethod
    def cleanup_expired(self, project_id: str) -> list[str]:
        """만료된 잠금을 정리합니다."""
        pass
```

## 기존 구현 예시

### SQLiteStateStore (기본)

SQLite 기반 저장소 (v1.1부터 기본값):

```python
# c4/store/sqlite.py

class SQLiteStateStore(StateStore):
    def __init__(self, db_path: Path):
        self.db_path = db_path
        self._init_db()

    def _get_connection(self):
        conn = sqlite3.connect(
            self.db_path,
            timeout=30.0,  # 락 대기 30초
        )
        # WAL 모드: 동시 읽기 허용, 멀티워커 안전
        conn.execute("PRAGMA journal_mode=WAL")
        conn.execute("PRAGMA busy_timeout=30000")
        return conn

    def load(self, project_id: str) -> C4State:
        with self._get_connection() as conn:
            if project_id:
                cursor = conn.execute(
                    "SELECT state_json FROM c4_state WHERE project_id = ?",
                    (project_id,),
                )
            else:
                # 단일 프로젝트 모드: 아무 프로젝트나 로드
                cursor = conn.execute("SELECT state_json FROM c4_state LIMIT 1")
            row = cursor.fetchone()

        if row is None:
            raise StateNotFoundError(f"State not found: {project_id}")

        return C4State.model_validate(json.loads(row[0]))

    def save(self, state: C4State) -> None:
        state.updated_at = datetime.now()
        with self._get_connection() as conn:
            conn.execute(
                "INSERT OR REPLACE INTO c4_state (project_id, state_json, updated_at) VALUES (?, ?, ?)",
                (state.project_id, state.model_dump_json(), state.updated_at),
            )
            conn.commit()
```

**WAL 모드 장점:**
- 동시 읽기 허용 (여러 Worker가 동시에 상태 조회 가능)
- 쓰기 충돌 시 자동 대기 (30초)
- Race condition 방지

### LocalFileStateStore (레거시)

파일 기반 저장소 (이전 기본값, 단일 Worker 전용):

```python
# c4/store/local_file.py

class LocalFileStateStore(StateStore):
    def __init__(self, c4_dir: Path):
        self.c4_dir = c4_dir
        self.state_file = c4_dir / "state.json"

    def load(self, project_id: str) -> C4State:
        if not self.state_file.exists():
            raise StateNotFoundError(f"State not found: {self.state_file}")

        data = json.loads(self.state_file.read_text())
        return C4State.model_validate(data)

    def save(self, state: C4State) -> None:
        state.updated_at = datetime.now()
        self.c4_dir.mkdir(parents=True, exist_ok=True)
        self.state_file.write_text(state.model_dump_json(indent=2))

    def exists(self, project_id: str) -> bool:
        return self.state_file.exists()

    def delete(self, project_id: str) -> None:
        if self.state_file.exists():
            self.state_file.unlink()
```

> **Note:** LocalFileStateStore는 단일 Worker 환경에서만 안전합니다.
> 멀티 Worker 환경에서는 SQLiteStateStore (기본값)를 사용하세요.

## 커스텀 Store 구현하기

### 예시: Redis StateStore

```python
# my_project/redis_store.py

import redis
from c4.store import StateStore, StateNotFoundError
from c4.models import C4State

class RedisStateStore(StateStore):
    def __init__(self, redis_url: str, prefix: str = "c4:state:"):
        self.client = redis.from_url(redis_url)
        self.prefix = prefix

    def _key(self, project_id: str) -> str:
        return f"{self.prefix}{project_id}"

    def load(self, project_id: str) -> C4State:
        data = self.client.get(self._key(project_id))
        if data is None:
            raise StateNotFoundError(f"State not found: {project_id}")

        return C4State.model_validate_json(data)

    def save(self, state: C4State) -> None:
        from datetime import datetime
        state.updated_at = datetime.now()
        self.client.set(
            self._key(state.project_id),
            state.model_dump_json()
        )

    def exists(self, project_id: str) -> bool:
        return self.client.exists(self._key(project_id)) > 0

    def delete(self, project_id: str) -> None:
        self.client.delete(self._key(project_id))
```

### 예시: Supabase StateStore

```python
# my_project/supabase_store.py

from supabase import create_client
from c4.store import StateStore, StateNotFoundError
from c4.models import C4State

class SupabaseStateStore(StateStore):
    def __init__(self, url: str, key: str):
        self.client = create_client(url, key)
        self.table = "c4_state"

    def load(self, project_id: str) -> C4State:
        result = (
            self.client.table(self.table)
            .select("state_json")
            .eq("project_id", project_id)
            .single()
            .execute()
        )

        if not result.data:
            raise StateNotFoundError(f"State not found: {project_id}")

        return C4State.model_validate_json(result.data["state_json"])

    def save(self, state: C4State) -> None:
        from datetime import datetime
        state.updated_at = datetime.now()

        self.client.table(self.table).upsert({
            "project_id": state.project_id,
            "state_json": state.model_dump_json(),
            "updated_at": state.updated_at.isoformat(),
        }).execute()

    def exists(self, project_id: str) -> bool:
        result = (
            self.client.table(self.table)
            .select("project_id")
            .eq("project_id", project_id)
            .execute()
        )
        return len(result.data) > 0

    def delete(self, project_id: str) -> None:
        self.client.table(self.table).delete().eq(
            "project_id", project_id
        ).execute()
```

## Store 사용하기

### 방법 1: Factory 사용 (권장)

C4는 내장된 Store Factory를 제공합니다:

```python
from pathlib import Path
from c4.store import create_state_store, create_lock_store

# 환경 변수 또는 기본값 사용
c4_dir = Path(".c4")
state_store = create_state_store(c4_dir)
lock_store = create_lock_store(c4_dir)
```

**환경 변수:**
```bash
# SQLite (기본값)
export C4_STORE_BACKEND=sqlite

# 로컬 파일 (레거시)
export C4_STORE_BACKEND=local_file

# Supabase
export C4_STORE_BACKEND=supabase
export SUPABASE_URL=https://xxx.supabase.co
export SUPABASE_KEY=your-anon-key
```

**config.yaml:**
```yaml
# .c4/config.yaml
project_id: my-project

# Store 설정
store:
  backend: supabase
  supabase_url: https://xxx.supabase.co
  supabase_key: your-anon-key
```

**설치:**
```bash
# Supabase 사용 시
uv add "c4[cloud]"
# 또는
uv add supabase
```

### 방법 2: 직접 주입

```python
from c4.mcp_server import C4Daemon
from my_project.redis_store import RedisStateStore

# 커스텀 Store 생성
store = RedisStateStore("redis://localhost:6379")

# C4Daemon에 주입
daemon = C4Daemon(project_root, state_store=store)
daemon.load()
```

### MCP 서버 설정

커스텀 Store를 사용하려면 MCP 서버 시작 스크립트를 수정합니다:

```python
# my_mcp_server.py

from c4.mcp_server import create_mcp_server
from my_project.redis_store import RedisStateStore

store = RedisStateStore("redis://localhost:6379")
server = create_mcp_server(state_store=store)

if __name__ == "__main__":
    import asyncio
    asyncio.run(server.run())
```

## 테스트

### Store 테스트 패턴

```python
# tests/test_my_store.py

import pytest
from my_project.redis_store import RedisStateStore
from c4.models import C4State
from c4.store import StateNotFoundError

@pytest.fixture
def store():
    return RedisStateStore("redis://localhost:6379/15")  # 테스트 DB

class TestRedisStateStore:
    def test_save_and_load(self, store):
        state = C4State(project_id="test")
        store.save(state)

        loaded = store.load("test")
        assert loaded.project_id == "test"

    def test_load_not_found(self, store):
        with pytest.raises(StateNotFoundError):
            store.load("nonexistent")

    def test_exists(self, store):
        assert store.exists("test") is False

        state = C4State(project_id="test")
        store.save(state)

        assert store.exists("test") is True

    def test_delete(self, store):
        state = C4State(project_id="test")
        store.save(state)
        store.delete("test")

        assert store.exists("test") is False
```

## 고려사항

### 1. 동시성
- 여러 Worker가 동시에 상태를 수정할 수 있음
- SQLite: WAL 모드로 동시 읽기 허용, busy_timeout으로 쓰기 충돌 처리
- 다른 Store: 낙관적 잠금 또는 트랜잭션 사용 권장

### 2. 성능
- 상태 크기가 클 수 있음 (많은 태스크, 이벤트)
- 압축 또는 분할 저장 고려

### 3. 마이그레이션
- 기존 LocalFile 데이터를 새 Store로 마이그레이션
- `load()` → 변환 → `save()` 패턴

### 4. 에러 처리
- 네트워크 오류, 타임아웃 처리
- 재시도 로직 구현

## 다음 단계

- [SupervisorBackend 확장](SupervisorBackend-확장.md) - 다른 LLM 연동
- [아키텍처](아키텍처.md) - 전체 시스템 구조
