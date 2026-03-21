# Python Style Guide

> Python 프로젝트 코딩 컨벤션.

## 도구 (필수)

- 패키지 관리: `uv` (pip install 절대 금지)
- 린트: `uv run ruff check .`
- 포매터: `uv run ruff format .`
- 타입 체크: `uv run mypy` (선택, 권장)
- 테스트: `uv run pytest`

## 실행 규칙

```bash
# 절대 금지
python script.py
pip install package
pytest

# 올바른 방법
uv run python script.py
uv add package
uv run pytest
```

## 패턴

- **Pathlib**: `os.path` 대신 `pathlib.Path` 사용.
- **Pydantic**: 데이터 검증에 Pydantic BaseModel 사용.
- **Type Hints**: 함수 시그니처에 타입 힌트 권장.
- **Context Manager**: 리소스 관리에 `with` 문 필수.
- **f-string**: 문자열 포매팅에 f-string 사용 (.format(), % 금지).

## FastAPI 규칙

- 라우터: `APIRouter` 사용, app에 직접 데코레이터 금지.
- 의존성 주입: `Depends()` 활용.
- 응답 모델: `response_model` 명시.
- 에러: `HTTPException` 사용, 적절한 status code.
- 비동기: I/O 바운드 엔드포인트는 `async def`.

## 프로젝트 구조

```
src/<package>/    소스 코드
tests/
  unit/           단위 테스트
  integration/    통합 테스트
pyproject.toml    프로젝트 설정 (setup.py 금지)
```

## 금지

- `*` import (`from module import *`).
- bare except (`except:` → `except Exception:`).
- mutable default argument (`def fn(lst=[])`).
- 글로벌 변수로 상태 관리.
