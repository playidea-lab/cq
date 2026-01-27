# LLM Provider 설정

C4는 Supervisor 리뷰에 다양한 LLM Provider를 사용할 수 있습니다. 기본값은 Claude Code (사용자 구독)이며, LiteLLM을 통해 100+ Provider를 지원합니다.

## 기본값: Claude Code

별도 설정 없이 Claude Code 구독을 사용합니다:

```yaml
# .c4/config.yaml
llm:
  model: claude-cli  # 기본값 (생략 가능)
```

**장점:**
- 별도 API 키 불필요
- Claude Code 구독으로 통합 관리
- 추가 비용 없음 (구독 내)

**사용량 확인:**
```bash
# 세션 내
/context
/cost

# CLI 도구
npx ccusage@latest daily
npx ccusage@latest monthly
```

## LiteLLM Provider

OpenAI, Anthropic API, Azure 등 다른 Provider를 사용하려면 LiteLLM을 통해 설정합니다.

### OpenAI

```yaml
llm:
  model: gpt-4o           # 또는 gpt-4o-mini, o1, o1-mini
  api_key_env: OPENAI_API_KEY
```

```bash
export OPENAI_API_KEY="sk-..."
```

### Anthropic API

Claude Code 구독이 아닌 Anthropic API를 직접 사용:

```yaml
llm:
  model: claude-3-opus-20240229  # 또는 claude-3-sonnet, claude-3-haiku
  api_key_env: ANTHROPIC_API_KEY
```

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

### Azure OpenAI

```yaml
llm:
  model: azure/gpt-4o-deployment  # deployment 이름
  api_base: https://my-resource.openai.azure.com
  api_key_env: AZURE_OPENAI_API_KEY
```

### Ollama (로컬)

로컬에서 LLM 실행:

```yaml
llm:
  model: ollama/llama3    # 또는 ollama/mistral, ollama/codellama
  api_base: http://localhost:11434
```

```bash
# Ollama 설치 및 실행
ollama pull llama3
ollama serve
```

### AWS Bedrock

```yaml
llm:
  model: bedrock/anthropic.claude-3-sonnet
  # AWS 자격 증명은 환경 변수 또는 ~/.aws/credentials 사용
```

### Google AI Studio (Gemini)

`gemini-pro`, `gemini-1.5-pro` 등을 Google AI Studio API 키로 사용:

```yaml
llm:
  model: gemini/gemini-1.5-pro-latest
  api_key_env: GOOGLE_API_KEY
```

```bash
# API 키 설정 (추천)
c4 config api-key set gemini
```

### Google Vertex AI

```yaml
llm:
  model: vertex_ai/gemini-pro
  # GCP 자격 증명 필요
```

### ZhipuAI (GLM)

```yaml
llm:
  model: zhipuai/glm-4    # 또는 glm-4-flash, chatglm-turbo
  api_key_env: ZHIPUAI_API_KEY
```

### Groq

```yaml
llm:
  model: groq/llama3-70b-8192
  api_key_env: GROQ_API_KEY
```

### Together AI

```yaml
llm:
  model: together_ai/meta-llama/Llama-3-70b
  api_key_env: TOGETHER_API_KEY
```

## 전체 설정 옵션

```yaml
llm:
  # 필수
  model: gpt-4o                    # LiteLLM 모델 식별자

  # API 인증
  api_key_env: OPENAI_API_KEY      # API 키 환경 변수 이름
  api_base: null                   # 커스텀 API 베이스 URL

  # 요청 설정
  timeout: 300                     # 타임아웃 (초, 30-600)
  max_retries: 3                   # 재시도 횟수 (1-10)
  temperature: 0.0                 # 샘플링 온도 (0.0-2.0)
  max_tokens: 4096                 # 최대 출력 토큰
  drop_params: true                # 지원하지 않는 파라미터 무시
```

## Provider 비교

| Provider | 모델 예시 | 특징 |
|----------|----------|------|
| **Claude Code** | claude-cli | 기본값, 구독 통합 |
| **OpenAI** | gpt-4o, o1 | 빠른 응답, 안정적 |
| **Anthropic API** | claude-3-opus | 긴 컨텍스트 |
| **Azure** | azure/* | 엔터프라이즈 보안 |
| **Ollama** | ollama/* | 로컬 실행, 무료 |
| **Bedrock** | bedrock/* | AWS 통합 |
| **Groq** | groq/* | 초고속 추론 |
| **ZhipuAI** | zhipuai/glm-* | 중국어 최적화 |

## 비용 관리

### LiteLLM 비용 추적

LiteLLM은 자동으로 비용을 추적합니다:

```python
# 프로그래밍 방식 접근
from c4.supervisor import LiteLLMBackend

backend = LiteLLMBackend(model="gpt-4o")
# ... run_review 후
if backend.last_usage:
    print(f"토큰: {backend.last_usage.total_tokens}")
    print(f"비용: ${backend.last_usage.cost:.4f}")
```

### 예상 비용 (2025년 1월 기준)

| 모델 | 입력 (1M 토큰) | 출력 (1M 토큰) |
|------|---------------|---------------|
| gpt-4o | $2.50 | $10.00 |
| gpt-4o-mini | $0.15 | $0.60 |
| claude-3-opus | $15.00 | $75.00 |
| claude-3-sonnet | $3.00 | $15.00 |
| llama3 (Ollama) | 무료 | 무료 |

## 트러블슈팅

### API 키 오류

```
SupervisorError: API key not found in environment variable: OPENAI_API_KEY
```

**해결:** 환경 변수 설정 확인
```bash
echo $OPENAI_API_KEY
export OPENAI_API_KEY="sk-..."
```

### 타임아웃

```
SupervisorError: Timeout after 300 seconds
```

**해결:** timeout 값 증가
```yaml
llm:
  model: gpt-4o
  timeout: 600  # 10분
```

### 모델 미지원 파라미터

```
Error: temperature is not supported
```

**해결:** `drop_params: true` 설정 (기본값)
```yaml
llm:
  model: o1
  drop_params: true  # 지원하지 않는 파라미터 무시
```

## 참고 자료

- [LiteLLM 문서](https://docs.litellm.ai/)
- [지원 Provider 전체 목록](https://docs.litellm.ai/docs/providers)
- [LiteLLM 모델 목록](https://models.litellm.ai/)
