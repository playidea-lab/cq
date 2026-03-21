# Infra Team Rules

> 인프라, DevOps, 플랫폼 엔지니어링 규칙.
> 적용: `cq init --team infra`

---

## IaC (Infrastructure as Code)

- 수동 콘솔 변경 금지. 모든 인프라는 코드로 관리.
- Terraform: 상태 파일은 remote backend (S3/GCS). 로컬 금지.
- 모듈화: 환경(dev/staging/prod)은 변수로 구분. 코드 복사 금지.
- plan → review → apply. `terraform apply` 직접 실행 금지.

## CI/CD

- 파이프라인 변경도 MR로 리뷰.
- 시크릿: CI variables 사용. 파이프라인 코드에 노출 금지.
- 배포: blue-green 또는 canary. 빅뱅 배포 금지.
- rollback: 1-command rollback 보장. 검증 주기적.

## 모니터링/알림

- 4대 신호: latency, traffic, errors, saturation.
- 알림: actionable한 것만. 알림 피로 금지.
- 대시보드: 서비스별 표준 대시보드. SLO 기반 임계치.
- on-call 런북: 알림마다 대응 절차 문서화.

## 보안

- 네트워크: 최소 권한 원칙. 필요한 포트만 개방.
- 시크릿: Vault 또는 동등 시크릿 매니저. 환경변수 직접 주입은 CI에서만.
- 이미지: base image 정기 업데이트. 취약점 스캔 자동화.
- 접근 제어: RBAC. 개인 계정에 직접 권한 부여 금지.

## Kubernetes (해당 시)

- manifest: Helm chart 또는 kustomize로 관리.
- 리소스 제한: requests/limits 필수. 미설정 배포 금지.
- PDB(PodDisruptionBudget): 프로덕션 서비스 필수.
- HPA: CPU/메모리 기반. 커스텀 메트릭은 검증 후.

## 문서

- runbook: 장애 시나리오별 대응 절차.
- 아키텍처: 서비스 의존성 다이어그램 최신 유지.
- 변경 로그: 인프라 변경은 changelog 기록 (Terraform plan 포함).
