C4 계획 단계 가이드 (Plan Stage Guide)

0. 계획 단계의 산출물(Definition of Done)

계획 단계가 “끝났다”는 조건은 아래가 모두 충족될 때다.
	•	전체 목표(Context7)가 **작업 단위(Work Items)**로 분해됨
	•	각 작업마다 Worker Packet 1개가 생성됨(1페이지 원칙)
	•	각 작업마다 **ContractSpec(동작 API 리스트 + 최소 테스트 리스트)**가 확정됨
	•	각 작업마다 **BoundaryMap(DDD 가드레일 요약)**이 포함됨
	•	각 작업마다 **Quality Gates(자동규칙)**가 명시됨
	•	작업 간 **의존성 그래프(DAG)**가 정의됨(선후관계)
	•	작업 크기가 기준 내로 쪼개졌음(“12일/13 API”)

⸻

1. 입력(Plan 단계가 받는 것)
	•	Goal/Outcome: “무엇이 바뀌어야 하는가”
	•	사용자 시나리오 / 운영 시나리오(핵심만)
	•	현재 As-Is 구조 스냅샷(As-Is LSP 또는 폴더/엔트리 요약)
	•	제약(보안/온프렘/성능/호환성/외부 연동 등)
	•	기존 코드 규칙(DDD 가드레일/클린코드 자동 규칙)

⸻

2. Plan 단계 절차(순서 고정)

Step 1) Outcome을 “외부 동작”으로 변환
	•	“무엇을 구현할지”를 API/행동으로 바꿔 적는다.
	•	API가 아니라면 “명령/이벤트/상태변화” 형태로 적는다.

규칙
	•	동작은 관찰 가능해야 함(입력→출력, 상태변화, 로그, 저장 등)
	•	내부 리팩토링/구현 방식은 적지 않는다.

⸻

Step 2) ContractSpec 작성(= 테스트 리스트 확정)

각 동작마다 최소 테스트를 확정한다.

필수 최소 세트
	•	성공 1
	•	실패 1
	•	경계/예외 1

추가(있으면 좋음)
	•	동시성/중복 요청(idempotency) 1
	•	권한/인증 1(해당 시)
	•	성능/타임아웃 1(해당 시)

이 단계가 끝나면 “완성 기준”이 생기고, 이후는 다 실행 문제다.

⸻

Step 3) 작업 분해(Work Breakdown)

동작(ContractSpec)을 기준으로 작업을 쪼갠다.

쪼개는 기준(필수)
	•	Packet 1개는 1~2일 내 끝날 크기
	•	Public API 1~3개
	•	테스트 3~9개 수준

쪼개는 트리거(쪼개야 하는 신호)
	•	파일/모듈을 5개 이상 건드릴 것 같다
	•	도메인 경계를 2개 이상 넘는다
	•	테스트가 10개 이상 필요해 보인다
	•	외부 연동까지 포함된다(= 별도 Packet)

⸻

Step 4) BoundaryMap 지정(DDD 가드레일 적용)

각 Work Item마다 다음을 확정한다.
	•	Target domain/context: 어디 도메인인가?
	•	Target layer: domain | app(usecase) | infra | api
	•	Forbidden imports: domain에서 금지되는 것
	•	Public export location: public API는 어디서만 노출?

Plan 단계 원칙
	•	DDD는 “완벽 모델링”이 아니라 경계/금지 규칙만 확정
	•	용어 사전의 적용 범위를 함께 명시(새 용어 추가 필요 여부)

⸻

Step 5) Code placement 고정(길 잃음 방지)

워커가 “어디에 뭘 만들지” 고민하지 않도록, 파일 위치를 확정한다.
	•	Create/Modify 파일 2~4개 수준으로 제한
	•	Tests 위치와 이름까지 지정

이게 세레나가 하는 ‘지도 고정’을 Packet에 내장하는 부분.

⸻

Step 6) Quality Gates 명시(클린코드 자동규칙)

작업 완료 조건(자동검사)을 Packet에 박는다.

필수
	•	format
	•	lint
	•	type-check
	•	tests
	•	forbidden import / cycle check

선택
	•	complexity 제한
	•	coverage 기준
	•	perf 기준

⸻

Step 7) 의존성(DAG) 정리 + 실행 순서 확정

각 Packet 간 선후관계를 정의한다.
	•	선행 작업이 끝나야 가능한 작업은 blocked_by로 표시
	•	병렬 가능한 작업은 병렬로 배치
	•	공통 타입/인터페이스 변경이 있으면 먼저 수행(충돌 최소화)

⸻

3. Plan 단계에서 “절대 하지 말아야 할 것”
	•	내부 클래스 구조를 완벽 설계하려고 하기
	•	DDD 용어(Aggregate 등)로 전부 분류하려고 하기
	•	코드 스타일(함수 쪼개기 등)까지 계획에 넣기
	•	To-Be LSP 전체 구조도를 만들기(대부분 과규격)

Plan은 “완성 기준 + 경계 + 파일 위치 + 게이트”까지만.

⸻

4. Worker Packet 작성 규칙(1페이지 원칙)

각 Packet에는 아래만 들어가면 된다.
	1.	Goal (한 줄)
	2.	ContractSpec (API 리스트 + 테스트 최소 세트)
	3.	BoundaryMap (레이어/금지 import/공개 위치)
	4.	Code placement (수정 파일 목록 + 테스트 위치)
	5.	Quality Gates
	6.	Checkpoints(CP1/2/3) 간단 정의

⸻

5. Plan 단계 리뷰 체크리스트(계획 검토용)
	•	이 Packet은 “무엇이 Done인지” 테스트로 말할 수 있나?
	•	경계/레이어가 명확한가(워커가 헷갈리지 않나)?
	•	파일 위치가 명확한가?
	•	작업 크기가 너무 크지 않나?
	•	병렬화/의존성 충돌이 최소화됐나?
	•	자동게이트로 품질이 강제되는가?

⸻

6. 예시(Plan 단계에서 작업 1개를 올바르게 만든 모습)
	•	Goal: “Job 생성 API가 queued 상태를 만든다”
	•	ContractSpec: create_job() + 테스트 3개(성공/실패/경계)
	•	BoundaryMap: job_queue 도메인, domain/app/api 경계, domain은 infra 금지
	•	Code placement: src/job_queue/domain/job.py, src/job_queue/app/create_job.py, src/api/jobs.py, tests/test_create_job.py
	•	Gates: format/lint/type/test + forbidden import


⸻

1) DDD 가드레일 (리뷰 규칙용)

DDD-1. 도메인 경계(Bounded Context) 간 직접 의존 금지
	•	서로 다른 도메인 모듈은 서로의 내부 타입/함수 import 금지
	•	허용: “공유 커널(shared types)” 또는 “인터페이스/DTO”를 통한 교류만

리뷰 체크: “A 도메인이 B 도메인 파일을 직접 import했는가?”

⸻

DDD-2. 도메인 레이어는 인프라를 모른다
	•	도메인 코드에서 금지:
	•	DB/ORM, HTTP 프레임워크, 클라우드 SDK, 파일시스템, 메시지큐 라이브러리 import
	•	도메인은 순수하게 규칙/정합성/상태 변화만

리뷰 체크: “domain/ 폴더에 infra 라이브러리가 들어왔는가?”

⸻

DDD-3. 애플리케이션(유즈케이스) 레이어에서 오케스트레이션, 도메인에서 규칙
	•	Application(Service/UseCase): 흐름(호출 순서, 트랜잭션, 외부 연동)
	•	Domain(Model/Policy): 비즈니스 규칙, 정합성, 불변 조건

리뷰 체크: “규칙이 컨트롤러/DB쿼리 안에 박혀 있나?”

⸻

DDD-4. Public API는 ‘한 군데’에서만 노출
	•	외부에서 import/호출해야 하는 것들은 api/ 또는 public/ 같은 단일 모듈에서 export
	•	나머지는 내부 구현으로 숨김

리뷰 체크: “외부에서 접근 가능한 엔트리가 여기저기 흩어졌나?”

⸻

DDD-5. 유비쿼터스 언어(용어) 최소 사전 고정
	•	핵심 용어 20~50개만 딱 고정 (예: Job, Run, Worker, Queue, Spec, Result…)
	•	같은 개념에 다른 이름 금지 / 같은 이름에 다른 의미 금지

리뷰 체크: “용어 충돌/동의어 난립이 생겼나?”

⸻

DDD-6. 도메인 이벤트/상태변화는 명시적으로
	•	중요한 상태 변화는 “이벤트/상태전이”로 표현 (이름만이라도)
	•	예: JobQueued, JobStarted, JobFailed, JobCompleted

리뷰 체크: “상태 변화가 여기저기에서 암묵적으로 일어나나?”

⸻

DDD-7. 엔티티/값 객체(Value Object) 최소 규칙
	•	불변 값(돈, 시간범위, ID 등)은 값 객체로 다루고, 흩어진 연산 금지
	•	엔티티는 식별자와 생명주기(상태변화)를 가진다

리뷰 체크: “금액/시간/ID 처리가 중복·산발적으로 퍼졌나?”

⸻

DDD-8. Anti-Corruption Layer(ACL)로 외부 시스템 격리
	•	외부 API/SDK 결과를 도메인 내부 타입으로 바로 들고 들어오지 말고 “변환 레이어”를 둠

리뷰 체크: “외부 응답 타입이 도메인 모델을 오염시키나?”

⸻

2) 클린코드 자동 규칙 (자동 검사로 강제)

아래는 “사람이 잔소리 안 해도 도구가 잡는” 규칙들로만 구성했어.

CC-1. 포맷 강제 (Formatter)
	•	Black(파이썬) / Prettier(TS) 같은 단일 포맷터로 통일
	•	리뷰에서 스타일 논쟁 금지

⸻

CC-2. 린트 강제 (Lint)
	•	Ruff(파이썬) / ESLint(TS)
	•	unused import, shadowing, 복잡도, 버그 패턴 자동 탐지

⸻

CC-3. 타입/스키마 체크 (Type check)
	•	Pyright/Mypy(파이썬), TS compiler strict
	•	“런타임 전에 잡을 수 있는 버그”를 사전에 차단

⸻

CC-4. 테스트 최소 기준 (Contract 중심)
	•	“핵심 public API”는 최소:
	•	성공 케이스 1개
	•	실패 케이스 1개
	•	경계값 1개
	•	테스트 없는 public API 추가 금지

⸻

CC-5. 복잡도 제한
	•	함수 cyclomatic complexity 상한(예: 10~15)
	•	분기 깊이 제한(예: 3)
	•	“거대한 if-else 덩어리” 방지

⸻

CC-6. 의존성 규칙 검사
	•	import cycle 금지
	•	forbidden import 규칙 (DDD-1,2를 도구로 강제)
	•	“폴더 간 의존”을 자동으로 막음

⸻

CC-7. Dead code / Deprecated 경로 강제 표기
	•	안 쓰는 코드/임시 코드는:
	•	deprecated/ 또는 experimental/로 격리
	•	또는 @deprecated/경고 주석 규칙
	•	“임시 코드가 프로덕션에 섞임” 방지

⸻

CC-8. 커밋/PR 게이트
	•	PR 통과 조건을 “자동 검사 통과”로 고정:
	•	format ✅
	•	lint ✅
	•	type ✅
	•	tests ✅
	•	사람이 하는 리뷰는 “의미/설계”만 보도록 만든다


Worker Packet Template (C4)

0. Task ID / Title
	•	ID: C4-XXXX
	•	Title: <한 줄 작업명>
	•	Owner(Worker): <agent/worker 이름>
	•	Related Context: <Context7/이슈/문서 링크 또는 요약>

⸻

1. Goal (What “Done” means)
	•	이 작업이 사용자/시스템에 주는 변화:
	•	<예: /v1/jobs/create 호출 시 Job이 queued 상태로 등록된다>
	•	Out of scope(이번 작업에서 하지 않는 것):
	•	<예: 스케줄링 정책 개선은 제외>

⸻

2. ContractSpec (동작 API 리스트 = 테스트 기준)

“이 목록이 통과하면 기능적으로 Done”

2.1 Public API
	1.	API/Func 이름

	•	Input: …
	•	Output: …
	•	Errors: …
	•	Side effects / State change: …

	2.	…

2.2 Required tests (최소 세트)
각 API마다:
	•	✅ success 1
	•	✅ failure 1
	•	✅ boundary/edge 1

⸻

3. Boundary Map (DDD 가드레일 요약)

“어디에 코드를 두고, 무엇을 import하면 안 되는지”

	•	Target domain/context: <예: job_queue>
	•	Target layer: domain | app(usecase) | infra | api
	•	Allowed imports:
	•	<예: domain -> stdlib, typing, pydantic types only>
	•	Forbidden imports:
	•	<예: domain -> db/orm/http/sdk 금지>
	•	Public export location:
	•	<예: src/api/__init__.py에서만 export>

⸻

4. Code placement (파일/디렉토리 지정)

“워커가 길 잃지 않게 ‘어디에 뭘 만들지’ 박아두기”

	•	Create/Modify:
	•	src/<...>/file_a.py : <역할>
	•	src/<...>/file_b.py : <역할>
	•	Tests:
	•	tests/<...>/test_*.py : <커버 대상>

⸻

5. Quality Gates (클린코드 자동 규칙)

“통과 못 하면 Done 불가 (자동화로 체크)”

	•	format ✅
	•	lint ✅
	•	type-check ✅
	•	tests ✅
	•	no forbidden imports / no cycles ✅
	•	(optional) complexity threshold ✅

⸻

6. Checkpoints (C4 루프 단계)

“중간 점검을 어디서 끊을지”

	•	CP1 Skeleton: 파일 배치 + 테스트 골격(실패해도 됨)
	•	CP2 Green: 핵심 API 1~2개 테스트 통과
	•	CP3 Harden: 에러/경계 케이스 확장 + 리팩토링 최소

⸻

7. Review prompts (피어리뷰 질문 5개)

리뷰어는 아래만 본다(스타일 논쟁 금지):
	1.	ContractSpec을 만족하는가?
	2.	경계(BoundaryMap)를 침범하지 않았는가?
	3.	Public API가 한 곳에서만 노출되는가?
	4.	테스트가 실패/경계 케이스를 포함하는가?
	5.	용어가 일관적인가?



자동 게이트를 실제 커맨드로 명시(예: ruff, pyright, pytest 등) 해서 “실행 가능한 패킷”으로 만들기
