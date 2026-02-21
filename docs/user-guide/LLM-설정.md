# LLM Gateway 설정

C4 LLM Gateway는 OpenAI, Anthropic, Gemini, Ollama 등 여러 LLM Provider를 통합 관리합니다.
`c4_llm_call`, Knowledge 임베딩, Permission Reviewer 모델 모드에서 사용됩니다.

---

## API 키 설정

### 권장: cq secret (암호화 저장)

```bash
cq secret set openai.api_key      # 터미널 echo 없음, 히스토리 미노출
cq secret set anthropic.api_key
cq secret set gemini.api_key

cq secret list                     # 저장된 키 이름 확인
```

- 저장 위치: `~/.c4/secrets.db` (AES-256-GCM 암호화, 전역 공유)
- CI 환경: `C4_MASTER_KEY=<64 hex chars>` 환경변수로 복호화 키 주입

### 키 해석 우선순위

```
config.yaml api_key  →  api_key_env (환경변수)  →  ~/.c4/secrets.db
```

세 방법 모두 지원되며 앞쪽이 우선합니다. 권장 방식은 `cq secret` 입니다.

---

## config.yaml 설정

```yaml
# .c4/config.yaml
llm_gateway:
  enabled: true
  default: openai           # 기본 provider
  cache_by_default: true    # Anthropic Prompt Caching 자동 적용

  providers:
    openai:
      enabled: true
      default_model: gpt-4o-mini
      # api_key는 cq secret set openai.api_key 로 저장 (자동 조회)

    anthropic:
      enabled: true
      # api_key는 cq secret set anthropic.api_key 로 저장

    gemini:
      enabled: false
      # api_key는 cq secret set gemini.api_key 로 저장

    ollama:
      enabled: false
      base_url: http://localhost:11434   # 로컬 Ollama 서버
```

---

## Provider별 설정

### OpenAI

```bash
cq secret set openai.api_key
```

```yaml
llm_gateway:
  default: openai
  providers:
    openai:
      enabled: true
      default_model: gpt-4o-mini   # gpt-4o, o1, o1-mini 등
```

### Anthropic

```bash
cq secret set anthropic.api_key
```

```yaml
llm_gateway:
  default: anthropic
  cache_by_default: true
  providers:
    anthropic:
      enabled: true
      default_model: claude-haiku-4-5-20251001
```

### Gemini

```bash
cq secret set gemini.api_key
```

```yaml
llm_gateway:
  providers:
    gemini:
      enabled: true
      default_model: gemini-1.5-flash
```

### Ollama (로컬, 키 불필요)

```yaml
llm_gateway:
  default: ollama
  providers:
    ollama:
      enabled: true
      base_url: http://localhost:11434
```

```bash
ollama pull llama3.2
ollama serve
```

---

## Knowledge 임베딩

`c4_knowledge_record`, `c4_knowledge_search` 등의 벡터 검색에 OpenAI 임베딩을 사용합니다.

```yaml
llm_gateway:
  enabled: true
  providers:
    openai:
      enabled: true
      # cq secret set openai.api_key 필요
```

- OpenAI 설정 시: 실제 임베딩 (1536차원, `text-embedding-3-small`)
- 미설정 시: Mock 임베딩 (384차원, FTS 전용)

---

## Permission Reviewer (model 모드)

Bash 명령 안전성 심사에 Anthropic Haiku를 사용하려면:

```yaml
# .c4/config.yaml
permission_reviewer:
  enabled: true
  mode: model              # "hook" (정규식만) or "model" (LLM 심사)
  model: haiku
  api_key_env: ANTHROPIC_API_KEY   # 환경변수 또는 cq secret 사용
```

`cq secret set anthropic.api_key` 저장 시 자동 조회되지 않습니다.
Permission Reviewer는 별도 환경변수(`api_key_env`) 방식을 유지합니다 (hook 프로세스 특성상).

---

## MCP 도구 직접 호출

```
c4_llm_call(prompt="...", provider="openai", model="gpt-4o")
c4_llm_providers()    # 활성화된 provider 목록
c4_llm_costs()        # 세션 내 누적 비용
```

---

## 트러블슈팅

### 키가 없을 때

```
cq: knowledge using mock embeddings (384d)
```
→ `cq secret set openai.api_key` 후 Claude Code 세션 재시작

### secret 저장 확인

```bash
cq secret list         # 저장된 키 이름 목록
cq secret get openai.api_key   # 값 확인
```

### CI 환경 설정

```bash
# 마스터 키 생성 (최초 1회)
export C4_MASTER_KEY=$(openssl rand -hex 32)

# 시크릿 저장
echo "sk-proj-..." | cq secret set openai.api_key

# 이후 실행 시 C4_MASTER_KEY 환경변수만 있으면 자동 복호화
```
