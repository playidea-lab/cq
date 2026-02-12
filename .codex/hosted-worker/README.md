# C4 Hosted Worker Plan (Codex 기준)

## 목적

`c4 run`과 별개로, 외부 프로세스(서비스/잡 워커)에서 C4 Worker 루프를 안정적으로 실행하는 운영 기준을 정의합니다.

핵심:
- C4 프로토콜은 그대로 사용 (`c4_get_task -> c4_submit`)
- LLM CLI는 provider adapter로 교체 가능
- 가격은 토큰비 + 인프라비 + 운영마진으로 산정

## 1) 지금 바로 가능한 구성 (c4-core 무수정)

1. Hosted 컨트롤러가 각 저장소마다 worker 프로세스 실행
2. 워커는 고정 `worker_id`로 `c4_get_task` polling
3. 구현/검증/commit 후 `c4_submit`
4. Direct 소유 태스크는 submit 에러 메시지 기반으로 skip

장점:
- 구현 속도 빠름
- 기존 Claude Code 시나리오와 충돌 없음

한계:
- heartbeat/lease가 없어 죽은 워커 복구가 느림
- submit 중복 방지(idempotency) 키 없음

## 2) 서비스 품질을 위한 권장 c4-core 확장

우선순위 P0:
1. `c4_get_task` lease TTL + heartbeat
2. `c4_submit` idempotency_key 지원
3. worker crash 시 자동 재큐잉(requeue) 정책

우선순위 P1:
1. task 실행 시간/토큰 사용량 필드 저장
2. provider/모델 정보 저장 (`executor=codex|claude|gemini|...`)
3. backoff hint (`retry_after_ms`) 반환

우선순위 P2:
1. quota/rate-limit 정책
2. org/project별 비용 한도

## 3) Provider 실행 가능성 (Worker Loop 관점)

판정 기준:
- 단일 커맨드 비대화식 실행 가능
- 종료코드 신뢰 가능
- 결과 캡처 가능(stdout/stderr)

현재 분류:
- Codex CLI: 높음 (권장 1순위)
- Claude Code: 중간 (버전별 비대화식 옵션 확인 필요)
- Gemini CLI: 중간 (버전별 단발 실행/JSON 옵션 확인 필요)
- OpenCode: 중간 (버전별 run/exec 인터페이스 확인 필요)

검증 커맨드:
```bash
bash .codex/tools/probe_worker_clis.sh
```

## 4) 가격 모델

단일 태스크 원가:
```text
model_cost = in_tokens * in_rate_per_1m / 1,000,000
           + out_tokens * out_rate_per_1m / 1,000,000

infra_cost = runtime_sec * cpu_usd_per_hour / 3600 + fixed_overhead

unit_cost = model_cost + infra_cost
price = unit_cost / (1 - margin)
```

샘플 계산:
```bash
python3 .codex/tools/hosted_pricing.py \
  --in-tokens 120000 --out-tokens 28000 \
  --in-rate 3.0 --out-rate 15.0 \
  --runtime-sec 420 --cpu-usd-per-hour 0.12 \
  --fixed-overhead 0.02 --margin 0.35
```

## 5) 운영 시나리오 (권장)

1. Stage 1 (2주)
- Codex 단일 provider로 hosted worker 파일럿
- Direct 태스크는 사람 세션에서만 처리

2. Stage 2 (4주)
- Claude/Gemini/OpenCode adapter 순차 추가
- provider별 성공률/재시도율 측정

3. Stage 3
- SLA 상품화(응답 시간, 월 task 한도, 우선순위 큐)

## 6) Go/No-Go 체크

- [ ] task submit 성공률 >= 95%
- [ ] 평균 재시도 횟수 <= 1.5
- [ ] unit_cost 대비 마진 목표 달성
- [ ] Direct/Worker ownership 충돌률 <= 2%
