import{_ as a,o as n,c as p,a2 as i}from"./chunks/framework.DdLJaIXX.js";const k=JSON.parse('{"title":"기능 계획: /pi → /plan → /run","description":"","frontmatter":{},"headers":[],"relativePath":"ko/examples/feature-planning.md","filePath":"ko/examples/feature-planning.md"}'),e={name:"ko/examples/feature-planning.md"};function l(t,s,h,o,d,r){return n(),p("div",null,[...s[0]||(s[0]=[i(`<h1 id="기능-계획-pi-→-plan-→-run" tabindex="-1">기능 계획: /pi → /plan → /run <a class="header-anchor" href="#기능-계획-pi-→-plan-→-run" aria-label="Permalink to &quot;기능 계획: /pi → /plan → /run&quot;">​</a></h1><p>CQ의 전체 계획 워크플로우를 사용하여 대형 기능 — 재시도 로직이 있는 실시간 Webhook 전달 — 을 구축하는 완전한 예제입니다.</p><hr><h2 id="이-워크플로우를-사용하는-경우" tabindex="-1">이 워크플로우를 사용하는 경우 <a class="header-anchor" href="#이-워크플로우를-사용하는-경우" aria-label="Permalink to &quot;이 워크플로우를 사용하는 경우&quot;">​</a></h2><p>다음과 같을 때 <code>/pi</code> → <code>/plan</code> → <code>/run</code>을 사용하세요:</p><ul><li>기능이 10개 이상의 파일에 영향을 미침</li><li>아키텍처 결정이 필요함 (DB 스키마, 재시도 전략, 큐 설계)</li><li>여러 엔지니어(또는 Worker)가 병렬로 작업함</li><li>코드 작성 전에 스펙 문서가 필요함</li></ul><p>더 작은 태스크에는 <code>/quick</code>을 사용하세요.</p><hr><h2 id="기능-webhook-전달-시스템" tabindex="-1">기능: Webhook 전달 시스템 <a class="header-anchor" href="#기능-webhook-전달-시스템" aria-label="Permalink to &quot;기능: Webhook 전달 시스템&quot;">​</a></h2><p><strong>목표</strong>: 이벤트가 발행될 때 (예: <code>job.completed</code>), 시스템이 등록된 모든 Webhook URL에 지수적 백오프 재시도(최대 5회, 재시도 간 2^n초)로 전달해야 합니다.</p><hr><h2 id="_1단계-pi로-아이디어-탐색" tabindex="-1">1단계: /pi로 아이디어 탐색 <a class="header-anchor" href="#_1단계-pi로-아이디어-탐색" aria-label="Permalink to &quot;1단계: /pi로 아이디어 탐색&quot;">​</a></h2><p>계획에 확정하기 전에 <code>/pi</code>로 브레인스토밍하고 설계 옵션을 검토합니다:</p><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>/pi &quot;재시도가 있는 Webhook 전달 시스템&quot;</span></span></code></pre></div><p>CQ가 Play Idea 모드에 진입합니다 — 저비용 탐색 단계입니다. 명확화 질문을 하고 옵션을 제시합니다:</p><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>[PI] 탐색 중: 재시도가 있는 Webhook 전달 시스템</span></span>
<span class="line"><span></span></span>
<span class="line"><span>범위 명확화를 위한 질문:</span></span>
<span class="line"><span>  1. 전달 순서: 엄격한 순서(FIFO) 또는 최선형?</span></span>
<span class="line"><span>  2. 재시도 저장소: 메모리 큐 또는 영속적 (SQLite/Postgres)?</span></span>
<span class="line"><span>  3. 실패 알림: 최종 실패 시 호출자에게 알려야 하나요?</span></span>
<span class="line"><span>  4. 동시성: 엔드포인트당 최대 병렬 전달 수?</span></span>
<span class="line"><span></span></span>
<span class="line"><span>설계 옵션:</span></span>
<span class="line"><span>  옵션 A: 인프로세스 goroutine 풀 + SQLite 재시도 테이블</span></span>
<span class="line"><span>    장점: 인프라 의존성 없음, 간단한 운영</span></span>
<span class="line"><span>    단점: 큐가 플러시되지 않으면 충돌 시 손실</span></span>
<span class="line"><span></span></span>
<span class="line"><span>  옵션 B: 별도의 webhook-worker 프로세스 + Redis 큐</span></span>
<span class="line"><span>    장점: 내구성, 확장성</span></span>
<span class="line"><span>    단점: Redis 필요, 복잡한 구성</span></span>
<span class="line"><span></span></span>
<span class="line"><span>  옵션 C: 이벤트 디스패처에 내장된 재시도 루프</span></span>
<span class="line"><span>    장점: 최소한의 코드, 새 테이블 없음</span></span>
<span class="line"><span>    단점: 디스패처 goroutine 차단</span></span>
<span class="line"><span></span></span>
<span class="line"><span>추천: v1에는 옵션 A (현재 SQLite 아키텍처에 적합)</span></span></code></pre></div><p>질문에 답합니다:</p><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>1. v1에서는 최선형으로 충분</span></span>
<span class="line"><span>2. 영속적 — SQLite, 나머지 시스템과 일관성 유지</span></span>
<span class="line"><span>3. 예, webhook.failed 이벤트 발생</span></span>
<span class="line"><span>4. 엔드포인트당 5개 동시</span></span></code></pre></div><p>CQ가 합의된 설계를 요약하고 계획으로 이동할 것을 제안합니다:</p><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>[PI] 계획 준비됨. /plan을 실행하여 태스크 생성.</span></span></code></pre></div><hr><h2 id="_2단계-plan으로-태스크-생성" tabindex="-1">2단계: /plan으로 태스크 생성 <a class="header-anchor" href="#_2단계-plan으로-태스크-생성" aria-label="Permalink to &quot;2단계: /plan으로 태스크 생성&quot;">​</a></h2><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>/plan &quot;Webhook 전달: SQLite 기반 재시도, 5회 시도, 지수 백오프, 소진 시 webhook.failed 이벤트&quot;</span></span></code></pre></div><p>CQ가 Discovery → Design → 태스크 생성을 실행합니다:</p><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>[DISCOVERY] 도메인: go backend</span></span>
<span class="line"><span>요구사항 수집 (EARS 형식):</span></span>
<span class="line"><span>  - job.completed 발생 시, 시스템이 등록된 모든 hook에 전달해야 함</span></span>
<span class="line"><span>  - 전달 실패 시, 시스템이 2^n초 지연으로 재시도해야 함 (최대 5회)</span></span>
<span class="line"><span>  - 모든 재시도 소진 시, 시스템이 webhook.failed 이벤트를 발생시켜야 함</span></span>
<span class="line"><span>  - 시스템이 엔드포인트당 최대 5개의 동시 전달을 지원해야 함</span></span>
<span class="line"><span></span></span>
<span class="line"><span>[DESIGN] 아키텍처 결정 기록 (ADR-014):</span></span>
<span class="line"><span>  저장소: webhooks 테이블 (id, url, event_type, created_at)</span></span>
<span class="line"><span>           webhook_deliveries 테이블 (id, webhook_id, payload, attempt, status, next_retry)</span></span>
<span class="line"><span>  Worker: WebhookWorker goroutine 풀 (풀 크기 = 엔드포인트당 5)</span></span>
<span class="line"><span>  재시도: 지수 백오프 1s, 2s, 4s, 8s, 16s</span></span>
<span class="line"><span></span></span>
<span class="line"><span>[TASKS] 6개 태스크 생성:</span></span>
<span class="line"><span>  T-WH-01  DB 스키마: webhooks + webhook_deliveries 테이블</span></span>
<span class="line"><span>  T-WH-02  WebhookStore: CRUD + pending-deliveries 쿼리</span></span>
<span class="line"><span>  T-WH-03  WebhookWorker: goroutine 풀 + HTTP 전달</span></span>
<span class="line"><span>  T-WH-04  RetryScheduler: 백오프 + 재큐 로직</span></span>
<span class="line"><span>  T-WH-05  EventBus 통합: job.completed 구독 → 큐에 추가</span></span>
<span class="line"><span>  T-WH-06  테스트: 단위 + 통합</span></span>
<span class="line"><span>  CP-WH    체크포인트: 모든 태스크 + 통합 테스트 통과</span></span>
<span class="line"><span></span></span>
<span class="line"><span>의존성:</span></span>
<span class="line"><span>  T-WH-02는 T-WH-01 필요</span></span>
<span class="line"><span>  T-WH-03는 T-WH-02 필요</span></span>
<span class="line"><span>  T-WH-04는 T-WH-03 필요</span></span>
<span class="line"><span>  T-WH-05는 T-WH-03 필요</span></span>
<span class="line"><span>  T-WH-06는 T-WH-03, T-WH-04, T-WH-05 필요</span></span>
<span class="line"><span>  CP-WH는 T-WH-06 필요</span></span></code></pre></div><p>태스크 목록 검토:</p><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>/status</span></span></code></pre></div><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>State: PLAN → EXECUTE</span></span>
<span class="line"><span>Queue: 6 pending | 0 in_progress | 0 done</span></span>
<span class="line"><span>지금 준비: T-WH-01 (의존성 없음)</span></span>
<span class="line"><span>차단됨: T-WH-02..06 (의존성 대기 중)</span></span></code></pre></div><hr><h2 id="_3단계-run으로-worker-실행" tabindex="-1">3단계: /run으로 Worker 실행 <a class="header-anchor" href="#_3단계-run으로-worker-실행" aria-label="Permalink to &quot;3단계: /run으로 Worker 실행&quot;">​</a></h2><p>Worker를 시작합니다. T-WH-01에 의존성이 없으므로 하나의 Worker가 즉시 시작할 수 있습니다:</p><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>/run 3</span></span></code></pre></div><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>3개의 Worker 스폰 중...</span></span>
<span class="line"><span></span></span>
<span class="line"><span>Worker-1 클레임: T-WH-01  DB 스키마: webhooks + webhook_deliveries 테이블</span></span>
<span class="line"><span>Worker-2: 준비된 태스크 없음, 대기 중...</span></span>
<span class="line"><span>Worker-3: 준비된 태스크 없음, 대기 중...</span></span></code></pre></div><p>Worker-1이 마이그레이션을 생성합니다:</p><div class="language-sql vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang">sql</span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span style="--shiki-light:#6A737D;--shiki-dark:#6A737D;">-- infra/supabase/migrations/00060_webhooks.sql</span></span>
<span class="line"><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">CREATE</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> TABLE</span><span style="--shiki-light:#6F42C1;--shiki-dark:#B392F0;"> webhooks</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;"> (</span></span>
<span class="line"><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">    id          </span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">TEXT</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> PRIMARY KEY</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> DEFAULT</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;"> gen_random_uuid(),</span></span>
<span class="line"><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">    url</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">         TEXT</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> NOT NULL</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">,</span></span>
<span class="line"><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">    event_type  </span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">TEXT</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> NOT NULL</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">,</span></span>
<span class="line"><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">    secret</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">      TEXT</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">,</span></span>
<span class="line"><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">    created_at  </span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">TIMESTAMPTZ</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> NOT NULL</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> DEFAULT</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> now</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">()</span></span>
<span class="line"><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">);</span></span>
<span class="line"></span>
<span class="line"><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">CREATE</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> TABLE</span><span style="--shiki-light:#6F42C1;--shiki-dark:#B392F0;"> webhook_deliveries</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;"> (</span></span>
<span class="line"><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">    id          </span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">TEXT</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> PRIMARY KEY</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> DEFAULT</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;"> gen_random_uuid(),</span></span>
<span class="line"><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">    webhook_id  </span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">TEXT</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> NOT NULL</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> REFERENCES</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;"> webhooks(id) </span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">ON DELETE CASCADE</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">,</span></span>
<span class="line"><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">    payload     JSONB </span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">NOT NULL</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">,</span></span>
<span class="line"><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">    attempt     </span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">INTEGER</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> NOT NULL</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> DEFAULT</span><span style="--shiki-light:#005CC5;--shiki-dark:#79B8FF;"> 0</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">,</span></span>
<span class="line"><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">    status</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">      TEXT</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> NOT NULL</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> DEFAULT</span><span style="--shiki-light:#032F62;--shiki-dark:#9ECBFF;"> &#39;pending&#39;</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">,  </span><span style="--shiki-light:#6A737D;--shiki-dark:#6A737D;">-- pending | delivered | failed</span></span>
<span class="line"><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">    next_retry  </span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">TIMESTAMPTZ</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">,</span></span>
<span class="line"><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">    created_at  </span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">TIMESTAMPTZ</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> NOT NULL</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> DEFAULT</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> now</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">()</span></span>
<span class="line"><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">);</span></span>
<span class="line"></span>
<span class="line"><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">CREATE</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> INDEX</span><span style="--shiki-light:#6F42C1;--shiki-dark:#B392F0;"> idx_webhook_deliveries_pending</span></span>
<span class="line"><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">    ON</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;"> webhook_deliveries(</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">status</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">, next_retry)</span></span>
<span class="line"><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;">    WHERE</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> status</span><span style="--shiki-light:#D73A49;--shiki-dark:#F97583;"> =</span><span style="--shiki-light:#032F62;--shiki-dark:#9ECBFF;"> &#39;pending&#39;</span><span style="--shiki-light:#24292E;--shiki-dark:#E1E4E8;">;</span></span></code></pre></div><p>Worker-1이 T-WH-01을 제출합니다. T-WH-02가 준비됩니다.</p><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>Worker-1이 T-WH-01 제출</span></span>
<span class="line"><span>Worker-2 클레임: T-WH-02  WebhookStore: CRUD + pending-deliveries 쿼리</span></span></code></pre></div><p>Worker들이 의존성이 해결되면서 계속 태스크를 가져갑니다. 진행 상황 확인:</p><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>/status</span></span></code></pre></div><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>State: EXECUTE</span></span>
<span class="line"><span>Queue: 2 pending | 3 in_progress | 1 done</span></span>
<span class="line"><span>  T-WH-01  [done]         DB 스키마</span></span>
<span class="line"><span>  T-WH-02  [in_progress]  WebhookStore</span></span>
<span class="line"><span>  T-WH-03  [in_progress]  WebhookWorker</span></span>
<span class="line"><span>  T-WH-04  [pending]      RetryScheduler    (T-WH-03에 의해 차단됨)</span></span>
<span class="line"><span>  T-WH-05  [pending]      EventBus 통합 (T-WH-03에 의해 차단됨)</span></span>
<span class="line"><span>  T-WH-06  [in_progress]  테스트</span></span>
<span class="line"><span>  CP-WH    [pending]      체크포인트</span></span></code></pre></div><hr><h2 id="_4단계-체크포인트-리뷰" tabindex="-1">4단계: 체크포인트 리뷰 <a class="header-anchor" href="#_4단계-체크포인트-리뷰" aria-label="Permalink to &quot;4단계: 체크포인트 리뷰&quot;">​</a></h2><p>모든 태스크가 완료되면 CQ가 자동으로 CHECKPOINT 상태에 진입합니다:</p><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>/status</span></span></code></pre></div><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>State: CHECKPOINT</span></span>
<span class="line"><span>Checkpoint: CP-WH — 6개 태스크 완료, 리뷰 대기 중</span></span></code></pre></div><p>리뷰 트리거:</p><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>/checkpoint</span></span></code></pre></div><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>[CHECKPOINT] CP-WH 리뷰 중...</span></span>
<span class="line"><span></span></span>
<span class="line"><span>변경사항 (6개 파일, +842줄, -12줄):</span></span>
<span class="line"><span>  infra/supabase/migrations/00060_webhooks.sql  (+38)</span></span>
<span class="line"><span>  internal/store/webhook_store.go               (+180)</span></span>
<span class="line"><span>  internal/worker/webhook_worker.go             (+210)</span></span>
<span class="line"><span>  internal/scheduler/retry_scheduler.go         (+95)</span></span>
<span class="line"><span>  internal/eventbus/dispatcher.go               (+45, -12)</span></span>
<span class="line"><span>  internal/worker/webhook_worker_test.go        (+274)</span></span>
<span class="line"><span></span></span>
<span class="line"><span>유효성 검사:</span></span>
<span class="line"><span>  go-build:       PASS</span></span>
<span class="line"><span>  go-vet:         PASS</span></span>
<span class="line"><span>  go-test:        PASS  (87개 테스트, 1.4s)</span></span>
<span class="line"><span>  integration:    PASS  (3개 테스트, 2.1s)</span></span>
<span class="line"><span></span></span>
<span class="line"><span>리뷰 결과: APPROVE</span></span>
<span class="line"><span></span></span>
<span class="line"><span>메모:</span></span>
<span class="line"><span>  - WebhookWorker가 엔드포인트별 동시성에 세마포어 올바르게 사용 (좋음)</span></span>
<span class="line"><span>  - RetryScheduler가 time.AfterFunc 사용 — 대형 큐에서는 향후 ticker 고려</span></span>
<span class="line"><span>  - maxAttempts에서 webhook.failed 이벤트 올바르게 발생</span></span></code></pre></div><hr><h2 id="_5단계-마무리" tabindex="-1">5단계: 마무리 <a class="header-anchor" href="#_5단계-마무리" aria-label="Permalink to &quot;5단계: 마무리&quot;">​</a></h2><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>/finish</span></span></code></pre></div><div class="language- vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang"></span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span>[FINISH] 다듬는 중...</span></span>
<span class="line"><span>  - 체인지로그 항목 생성</span></span>
<span class="line"><span>  - 최종 유효성 검사 실행</span></span>
<span class="line"><span>  - 모든 검사 통과</span></span>
<span class="line"><span></span></span>
<span class="line"><span>State: COMPLETE</span></span>
<span class="line"><span></span></span>
<span class="line"><span>요약: Webhook 전달 시스템</span></span>
<span class="line"><span>  - 6개 태스크 완료</span></span>
<span class="line"><span>  - 842줄 추가</span></span>
<span class="line"><span>  - 87개 단위 + 3개 통합 테스트 통과</span></span>
<span class="line"><span>  - 릴리즈 준비 완료</span></span></code></pre></div><hr><h2 id="각-단계가-하는-일" tabindex="-1">각 단계가 하는 일 <a class="header-anchor" href="#각-단계가-하는-일" aria-label="Permalink to &quot;각 단계가 하는 일&quot;">​</a></h2><table tabindex="0"><thead><tr><th>단계</th><th>커맨드</th><th>목적</th></tr></thead><tbody><tr><td>탐색</td><td><code>/pi &quot;아이디어&quot;</code></td><td>확정 전 옵션 브레인스토밍, 제약사항 명확화</td></tr><tr><td>계획</td><td><code>/plan &quot;스펙&quot;</code></td><td>스펙, 설계 결정(ADR), 태스크 큐 생성</td></tr><tr><td>실행</td><td><code>/run N</code></td><td>N개의 Worker 스폰; 의존성 해결되면 태스크 처리</td></tr><tr><td>리뷰</td><td><code>/checkpoint</code></td><td>관리자가 모든 변경사항 검토; 승인 또는 변경 요청</td></tr><tr><td>마무리</td><td><code>/finish</code></td><td>다듬기, 체인지로그, 최종 유효성 검사</td></tr></tbody></table><hr><h2 id="팁" tabindex="-1">팁 <a class="header-anchor" href="#팁" aria-label="Permalink to &quot;팁&quot;">​</a></h2><p><strong>Worker 수 조정</strong>: <code>/run 3</code>이 좋은 기본값입니다. 독립적인 태스크가 많으면 Worker가 더 많을수록 도움이 됩니다. 선형 의존성 체인(A → B → C)에서는 추가 Worker가 유휴 상태가 됩니다.</p><p><strong>체크포인트에서 변경 요청 시</strong>: CQ가 새 태스크를 생성하고 EXECUTE로 돌아갑니다. Worker들이 자동으로 처리합니다 — 기다리거나 <code>/run</code>을 다시 실행하세요.</p><p><strong>스펙은 보존됩니다</strong>: <code>/plan</code>의 스펙과 설계는 <code>.c4/specs/</code>와 <code>.c4/designs/</code>에 저장됩니다. <code>c4_get_spec</code> 또는 <code>c4_get_design</code>으로 언제든지 참조할 수 있습니다.</p><hr><h2 id="다음-단계" tabindex="-1">다음 단계 <a class="header-anchor" href="#다음-단계" aria-label="Permalink to &quot;다음 단계&quot;">​</a></h2><ul><li><strong>GPU 워크로드</strong>: <a href="./research-loop.html">연구 루프</a></li><li><strong>연구자 워크플로우</strong>: <a href="./research-loop.html">연구 루프</a></li><li><strong>전체 워크플로우 레퍼런스</strong>: <a href="./../reference/commands.html">사용 가이드</a></li></ul>`,63)])])}const g=a(e,[["render",l]]);export{k as __pageData,g as default};
