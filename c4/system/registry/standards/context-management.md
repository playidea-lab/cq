# Context Window Management

> 컨텍스트 윈도우를 효율적으로 관리하기 위한 가이드라인입니다.

## 핵심 제한사항

| 항목 | 권장 | 최대 | 초과 시 |
|------|------|------|---------|
| **MCP 서버** | 5-8개 | 10개 | 컨텍스트 급감 |
| **총 도구 수** | 50개 | 80개 | 응답 품질 저하 |

---

## 왜 중요한가?

```
모든 MCP 서버 활성화 시:
  200k 토큰 → 70k 토큰 (65% 감소!)

10개 이하 MCP 유지 시:
  200k 토큰 → 150k+ 토큰 유지 가능
```

**컨텍스트가 부족하면:**
- 이전 대화 내용 망각
- 코드 분석 품질 저하
- 복잡한 작업 실패 가능성 증가

---

## MCP 서버 우선순위

### 필수 (항상 활성화)

| 서버 | 용도 |
|------|------|
| **c4** | 프로젝트 관리, 태스크 오케스트레이션 |
| **serena** | 코드베이스 분석, 심볼릭 편집 |

### 권장 (프로젝트 유형별)

| 프로젝트 유형 | 추가 권장 MCP |
|--------------|--------------|
| **웹 프론트엔드** | filesystem, browser |
| **백엔드 API** | database, docker |
| **ML/DL** | jupyter, data-profiler |
| **인프라** | terraform, kubernetes |

### 선택적 (필요시만)

| 서버 | 언제 활성화 |
|------|------------|
| **github** | PR/Issue 작업 시 |
| **memory** | 장기 컨텍스트 필요 시 |
| **context7** | 외부 라이브러리 문서 참조 시 |

---

## 도구 수 관리

### 카운트 방법

```bash
# 현재 활성화된 도구 수 확인
# Claude Code에서 /tools 명령어 사용
```

### 80개 초과 시 대응

1. **불필요한 MCP 비활성화**
   ```json
   // .claude/settings.json
   {
     "disabledMcpjsonServers": ["unused-server"]
   }
   ```

2. **프로젝트별 MCP 구성**
   - 각 프로젝트에 필요한 MCP만 `.mcp.json`에 정의
   - `enableAllProjectMcpServers: false`로 수동 승인

3. **세션 분리**
   - 복잡한 작업은 별도 세션에서 최소 MCP로 진행

---

## 설정 예시

### 최소 구성 (권장)

```json
// .mcp.json
{
  "mcpServers": {
    "c4": { "command": "uv", "args": ["run", "c4-mcp"] },
    "serena": { "command": "uv", "args": ["run", "serena-mcp"] }
  }
}
```

### 프로젝트별 확장

```json
// .mcp.json (웹 프로젝트)
{
  "mcpServers": {
    "c4": { "..." },
    "serena": { "..." },
    "filesystem": { "..." },
    "browser": { "..." }
  }
}
// 총 4개 - 권장 범위 내
```

---

## 체크리스트

세션 시작 시:
- [ ] 활성화된 MCP 서버 10개 이하?
- [ ] 총 도구 수 80개 이하?
- [ ] 프로젝트에 불필요한 MCP 없는지?

컨텍스트 부족 증상 시:
- [ ] 이전 대화 내용을 자주 잊는가?
- [ ] 복잡한 코드 분석이 실패하는가?
- [ ] 긴 파일 읽기가 중단되는가?

→ MCP 서버 줄이고 세션 재시작 권장

---

## 참고

- [everything-claude-code 분석](https://github.com/affaan-m/everything-claude-code)
- 원문: "Context window management is important. With all MCP enabled, context drops from 200k to 70k tokens."
