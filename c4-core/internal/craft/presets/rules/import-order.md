# Rule: import-order
> import 순서 규칙. stdlib → external → internal 순서로 그룹화, 각 그룹 사이 빈 줄.

## 규칙

순서: **1. 표준 라이브러리** → **2. 외부 패키지** → **3. 내부/로컬 패키지**

각 그룹 사이에 빈 줄 1개. 그룹 내부는 알파벳 순서.

## Go

```go
import (
    // 1. 표준 라이브러리
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    // 2. 외부 패키지
    "github.com/go-chi/chi/v5"
    "go.uber.org/zap"

    // 3. 내부 패키지
    "github.com/myorg/myapp/internal/auth"
    "github.com/myorg/myapp/internal/model"
)
```

goimports 또는 `gofumpt`를 사용하면 자동 정렬된다.

## TypeScript / JavaScript

```typescript
// 1. Node.js 내장 모듈
import path from 'path';
import fs from 'fs/promises';

// 2. 외부 패키지
import express from 'express';
import { z } from 'zod';

// 3. 내부 모듈 (절대 경로)
import { AuthService } from '@/services/auth';
import { UserRepository } from '@/repositories/user';

// 4. 상대 경로 (같은 패키지)
import { validateInput } from './validators';
import type { RequestContext } from './types';
```

ESLint `import/order` 플러그인으로 자동 강제 가능.

## Python

```python
# 1. 표준 라이브러리
import os
import sys
from pathlib import Path
from typing import Optional

# 2. 외부 패키지
import httpx
from pydantic import BaseModel

# 3. 내부 패키지
from myapp.auth import authenticate
from myapp.models import User
```

`isort` 또는 `ruff --select I`로 자동 정렬.

## 도구 설정

### Go — golangci-lint
```yaml
linters-settings:
  goimports:
    local-prefixes: github.com/myorg/myapp
```

### TypeScript — ESLint
```json
"import/order": ["error", {
  "groups": ["builtin", "external", "internal", "parent", "sibling"],
  "newlines-between": "always"
}]
```

### Python — ruff
```toml
[tool.ruff.lint.isort]
known-first-party = ["myapp"]
```

# CUSTOMIZE: 내부 패키지 경로 설정
# 예: Go local-prefixes (조직 모듈 경로)
# 예: TypeScript path alias (@/ 설정)
# 예: Python first-party 패키지명
