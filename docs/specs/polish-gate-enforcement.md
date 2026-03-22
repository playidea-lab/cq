feature: Polish Gate Enforcement — 하이브리드 품질 수렴 강제
domain: web-backend
requirements:
  ubiquitous:
    - c4_submit은 diff 크기가 threshold 이상일 때 c4_gates에 polish=done 기록이 있는지 확인해야 한다
    - polish gate 미통과 시 submit을 거부하고 사유를 반환해야 한다
    - diff 크기가 threshold 미만(trivial)이면 자동 pass해야 한다
    - checkpoint_mode 설정(auto/interactive)이 config.yaml에서 읽히고 Go에서 동작해야 한다
  event_driven:
    - WHEN 워커가 구현 완료하면 THEN 자체 review agent를 스폰하고 polish 루프를 실행한다
    - WHEN polish 수렴 시 THEN c4_gates에 기록한다
  state_driven:
    - WHILE checkpoint_mode=auto일 때 THEN 워커가 자율적으로 polish+checkpoint 처리한다
    - WHILE checkpoint_mode=interactive일 때 THEN polish 후 사용자 확인을 대기한다
  unwanted:
    - 5줄 미만 변경에 polish 강제 (false block)
    - 기존 R- 리뷰 태스크 플로우 변경
    - /c4-finish 스킬 폐기
out_of_scope:
    - persona 학습 기반 자동 threshold 조정 (Phase 3)
    - auto→interactive 전환 로직 (Phase 3)
    - critique loop Go 구현