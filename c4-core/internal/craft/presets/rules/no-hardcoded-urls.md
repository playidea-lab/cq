# Rule: no-hardcoded-urls
> URL, IP 주소, 포트를 코드에 하드코딩 금지. 환경변수 또는 설정 파일을 사용한다.

## 규칙

- 코드에 `http://`, `https://`, IP 주소, 도메인을 직접 삽입 금지
- 개발/스테이징/프로덕션 URL을 조건 분기로 하드코딩 금지
- 포트 번호 하드코딩 금지 (설정 파일에서 읽기)

## 금지 패턴

```go
// 금지
const apiURL = "https://api.production.com"
resp, _ := http.Get("http://localhost:8080/health")

// 금지
if env == "prod" {
    url = "https://api.production.com"
} else {
    url = "http://localhost:3000"
}
```

```python
# 금지
API_URL = "https://api.example.com/v1"
DB_HOST = "192.168.1.100"
```

## 허용 패턴

```go
// 허용 — 환경변수
apiURL := os.Getenv("API_URL")
if apiURL == "" {
    apiURL = "http://localhost:8080" // 개발 기본값만 허용
}

// 허용 — 설정 구조체
type Config struct {
    APIURL string `env:"API_URL" envDefault:"http://localhost:8080"`
}
```

```python
# 허용
import os
API_URL = os.environ["API_URL"]  # 기본값 없이 필수 강제
# 또는
API_URL = os.getenv("API_URL", "http://localhost:8080")  # 개발 기본값
```

## 예외

- 테스트 코드에서 `localhost` 사용 허용
- `localhost`/`127.0.0.1` 기본값은 개발 환경 한정 허용
- 공개 API 엔드포인트 (예: `https://api.github.com`)는 상수로 허용

## 탐지 방법

```bash
grep -rn "https\?://[^\"']\+\." --include="*.go" --include="*.py" --include="*.ts" . \
  | grep -v "_test\.\|test_\|localhost\|127.0.0.1"
```

# CUSTOMIZE: 허용하는 외부 공개 URL, 설정 로더 라이브러리 (viper, godotenv, pydantic-settings)
