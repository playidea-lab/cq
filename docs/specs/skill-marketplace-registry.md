feature: Skill Marketplace Registry — Supabase 기반 중앙 레지스트리
domain: cli
stakeholders: [CQ 사용자, CQ 관리자]

requirements:
  ubiquitous:
    - U1: 스킬 레지스트리는 Supabase skill_registry 테이블에 메타데이터를 저장한다
    - U2: 버전 히스토리는 skill_registry_versions 테이블에 저장한다
    - U3: 설치된 스킬은 `# source: cq:<name>@<version>` 주석으로 출처를 추적한다
    - U4: 인증은 기존 Supabase Auth (GitHub OAuth, cq auth login)를 사용한다

  event_driven:
    - E1: WHEN `cq publish <path>` 실행하면 THEN 로컬 스킬을 Supabase에 업로드한다
    - E2: WHEN 같은 이름의 스킬이 이미 존재하면 THEN semver bump 후 새 버전으로 등록한다
    - E3: WHEN `cq add <name>` 실행하면 THEN Supabase 레지스트리 → 빌트인 → GitHub 순으로 검색한다
    - E4: WHEN `cq search <query>` 실행하면 THEN 레지스트리에서 이름/설명 FTS 검색 결과를 반환한다
    - E5: WHEN 설치 성공하면 THEN download_count를 +1 증가시킨다
    - E6: WHEN `cq update <name>`이고 source가 `cq:` prefix이면 THEN 레지스트리에서 최신 버전을 가져온다
    - E7: WHEN 로컬 버전과 레지스트리 최신이 동일하면 THEN "이미 최신" 메시지를 표시한다

  optional:
    - O1: IF 관리자가 아닌 사용자가 publish 시도하면 THEN "관리자 전용" 에러를 반환한다 (Phase 1)

  unwanted:
    - W1: WHEN 네트워크 실패 시 THEN 빌트인 embed 프리셋으로 fallback한다 (설치 실패 금지)
    - W2: WHEN Supabase 장애 시 THEN 기존 GitHub/빌트인 경로는 영향받지 않는다

non_functional:
  - 레지스트리 검색 응답 2초 이내
  - 설치는 기존 빌트인과 동일한 UX (추가 단계 없음)
  - 오프라인 시 빌트인 fallback 투명하게 동작

out_of_scope:
  - 인기도 랭킹 알고리즘
  - 리뷰/평점 시스템
  - 결제/유료 스킬
  - 런타임 샌드박싱
  - 사용자 publish (Phase 2)
  - verified 뱃지 (Phase 3)

verification:
  - cli: cq publish, cq add, cq search, cq update 명령 E2E
  - http: Supabase REST API 호출 정상 동작
