# Coding Style Guide

> 모든 코드 작성 시 적용되는 스타일 가이드입니다.

## 파일 크기 제한

| 구분 | 권장 | 최대 | 초과 시 |
|------|------|------|---------|
| **파일** | 200-400줄 | 500줄 | 분할 필수 |
| **함수/메서드** | 30줄 | 50줄 | 리팩토링 필수 |
| **클래스** | 200줄 | 300줄 | 분할 고려 |

```python
# ❌ BAD: 500줄 초과 파일
# services/giant_service.py (800줄)

# ✅ GOOD: 논리적 단위로 분할
# services/user_service.py (150줄)
# services/auth_service.py (120줄)
# services/notification_service.py (100줄)
```

---

## Python 명명 규칙

### 변수 및 함수: `snake_case`

```python
# ✅ GOOD
user_name = "john"
total_count = 42
max_retry_attempts = 3

def calculate_total_price(items: list) -> float:
    pass

def get_user_by_id(user_id: int) -> User:
    pass

# ❌ BAD
userName = "john"        # camelCase
TotalCount = 42          # PascalCase
def CalculatePrice():    # PascalCase
    pass
```

### 클래스: `PascalCase`

```python
# ✅ GOOD
class UserService:
    pass

class DatabaseConnection:
    pass

class HTTPClientError(Exception):  # 약어는 대문자 유지
    pass

# ❌ BAD
class user_service:      # snake_case
    pass

class Httpclenterror:    # 약어 일관성 없음
    pass
```

### 상수: `SCREAMING_SNAKE_CASE`

```python
# ✅ GOOD
MAX_CONNECTIONS = 100
DEFAULT_TIMEOUT_SECONDS = 30
API_BASE_URL = "https://api.example.com"

# ❌ BAD
maxConnections = 100     # camelCase
default_timeout = 30     # 상수인데 소문자
```

### Private 멤버: `_single_underscore`

```python
class UserService:
    def __init__(self):
        self._cache = {}           # ✅ private 속성
        self.__secret = "key"      # ⚠️ name mangling, 특별한 경우만

    def _validate_input(self):     # ✅ private 메서드
        pass
```

---

## TypeScript 명명 규칙

### 변수 및 함수: `camelCase`

```typescript
// ✅ GOOD
const userName = "john";
const totalCount = 42;

function calculateTotalPrice(items: Item[]): number {
  return items.reduce((sum, item) => sum + item.price, 0);
}

const getUserById = async (userId: number): Promise<User> => {
  // ...
};

// ❌ BAD
const user_name = "john";   // snake_case
function CalculatePrice() {} // PascalCase
```

### 인터페이스/타입: `PascalCase`

```typescript
// ✅ GOOD
interface UserProfile {
  id: number;
  name: string;
  email: string;
}

type ApiResponse<T> = {
  data: T;
  error?: string;
};

// ❌ BAD
interface userProfile {}   // camelCase
type api_response = {};    // snake_case
```

### 상수: `SCREAMING_SNAKE_CASE` 또는 `camelCase`

```typescript
// ✅ GOOD - 환경 상수
const MAX_CONNECTIONS = 100;
const API_BASE_URL = "https://api.example.com";

// ✅ GOOD - 일반 상수 (camelCase도 허용)
const defaultTimeout = 30;
const configOptions = { retries: 3 };

// ❌ BAD
const max_connections = 100;  // snake_case 비권장
```

### Enum: `PascalCase`

```typescript
// ✅ GOOD
enum UserRole {
  Admin = "ADMIN",
  User = "USER",
  Guest = "GUEST",
}

enum HttpStatus {
  Ok = 200,
  NotFound = 404,
  InternalError = 500,
}

// ❌ BAD
enum user_role {}  // snake_case
```

---

## 불변성 (Immutability)

### Python

```python
from typing import Final
from dataclasses import dataclass, field

# ✅ GOOD: 상수는 Final로 명시
MAX_RETRIES: Final = 3

# ✅ GOOD: 불변 데이터 클래스
@dataclass(frozen=True)
class UserConfig:
    name: str
    max_connections: int = 10

# ✅ GOOD: 튜플 사용 (불변 시퀀스)
ALLOWED_EXTENSIONS: Final = (".py", ".ts", ".js")

# ❌ BAD: 가변 기본값
def process(items: list = []):  # 위험!
    items.append("new")
    return items

# ✅ GOOD: None 기본값 + 내부 초기화
def process(items: list | None = None):
    if items is None:
        items = []
    items.append("new")
    return items
```

### TypeScript

```typescript
// ✅ GOOD: const 사용
const MAX_RETRIES = 3;

// ✅ GOOD: readonly 속성
interface UserConfig {
  readonly name: string;
  readonly maxConnections: number;
}

// ✅ GOOD: as const로 리터럴 타입
const ALLOWED_EXTENSIONS = [".py", ".ts", ".js"] as const;

// ✅ GOOD: Readonly 유틸리티 타입
function processConfig(config: Readonly<UserConfig>) {
  // config.name = "new"; // 에러: readonly
}

// ❌ BAD: let 남용
let count = 0; // 재할당 필요 없으면 const 사용
```

---

## 타입 힌트

### Python

```python
from typing import Optional, Union, TypeVar, Generic
from collections.abc import Callable, Iterable

# ✅ GOOD: 함수 시그니처에 타입 힌트
def get_user_by_id(user_id: int) -> User | None:
    """사용자 ID로 사용자 조회."""
    pass

def process_items(
    items: list[str],
    transformer: Callable[[str], str] | None = None,
) -> list[str]:
    """아이템 목록 처리."""
    pass

# ✅ GOOD: 클래스 속성 타입
class UserService:
    def __init__(self, db: Database) -> None:
        self._db: Database = db
        self._cache: dict[int, User] = {}

# ✅ GOOD: 제네릭 타입
T = TypeVar("T")

class Repository(Generic[T]):
    def get(self, id: int) -> T | None:
        pass

    def save(self, entity: T) -> T:
        pass

# ❌ BAD: 타입 힌트 없음
def get_user(id):  # 반환 타입 불명확
    pass
```

### TypeScript

```typescript
// ✅ GOOD: 함수 시그니처에 타입
function getUserById(userId: number): User | null {
  // ...
}

// ✅ GOOD: 제네릭 함수
function firstOrDefault<T>(items: T[], defaultValue: T): T {
  return items.length > 0 ? items[0] : defaultValue;
}

// ✅ GOOD: 유니온 타입
type Result<T> = { success: true; data: T } | { success: false; error: string };

// ❌ BAD: any 타입 사용
function processData(data: any): any {
  // 타입 안전성 없음
}

// ✅ GOOD: unknown + 타입 가드
function processData(data: unknown): string {
  if (typeof data === "string") {
    return data.toUpperCase();
  }
  throw new Error("Expected string");
}
```

---

## 코드 구조

### 임포트 순서

**Python:**
```python
# 1. 표준 라이브러리
import os
import sys
from pathlib import Path

# 2. 서드파티 라이브러리
import pytest
from pydantic import BaseModel

# 3. 로컬 모듈
from c4.models import Task
from c4.services import TaskService
```

**TypeScript:**
```typescript
// 1. Node.js 내장 모듈
import path from "path";
import fs from "fs";

// 2. 외부 패키지
import express from "express";
import { z } from "zod";

// 3. 내부 모듈 (절대 경로)
import { UserService } from "@/services/user";

// 4. 상대 경로
import { formatDate } from "./utils";
```

### 함수/메서드 구조

```python
def process_order(
    order: Order,
    *,  # keyword-only 강제
    validate: bool = True,
    notify: bool = True,
) -> ProcessResult:
    """주문 처리.

    Args:
        order: 처리할 주문
        validate: 유효성 검사 여부
        notify: 알림 발송 여부

    Returns:
        처리 결과

    Raises:
        ValidationError: 유효성 검사 실패 시
    """
    # 1. 입력 검증 (Early return)
    if validate and not order.is_valid():
        raise ValidationError("Invalid order")

    # 2. 핵심 로직
    result = _execute_order(order)

    # 3. 부가 작업
    if notify:
        _send_notification(order, result)

    # 4. 반환
    return result
```

---

## 위반 시 처리

```
코드 작성
    ↓
파일 크기 검사 ──→ 500줄 초과 → ❌ 리뷰 차단
    ↓
함수 크기 검사 ──→ 50줄 초과 → ⚠️ 리팩토링 권고
    ↓
명명 규칙 검사 ──→ 위반 → 💡 수정 권고
    ↓
타입 힌트 검사 ──→ 누락 → 💡 추가 권고
    ↓
✅ 통과
```

---

## 자동 검사 (Lint)

```bash
# Python
uv run ruff check --fix .
uv run ruff format .
uv run mypy .

# TypeScript
npx eslint --fix .
npx prettier --write .
npx tsc --noEmit
```

---

## 참고 자료

- [PEP 8 - Python Style Guide](https://peps.python.org/pep-0008/)
- [Google TypeScript Style Guide](https://google.github.io/styleguide/tsguide.html)
- [Ruff - Python Linter](https://docs.astral.sh/ruff/)
