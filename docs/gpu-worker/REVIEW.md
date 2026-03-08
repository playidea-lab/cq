# C5 Worker UX 코드 리뷰

리뷰 대상: `c4-core/cmd/c4/hub_worker.go`, `c5/cmd/c5/worker.go`, `c5/internal/store/sqlite.go`
코드 기준: v0.83.0 (read-only)

---

## hub_worker.go

### API key 평문 저장 — 심각도: MEDIUM

재현 조건:
- `cq hub worker init` 실행 시 API key가 `~/.c5/config.yaml`에 YAML 평문으로 저장된다.
- 파일 퍼미션은 `0o600`으로 올바르게 설정되어 있어 다른 OS 사용자로부터는 보호된다.
- 그러나 `cat ~/.c5/config.yaml` 으로 누구나(해당 사용자라면) 즉시 확인 가능하며,
  `git add` 실수, 백업 도구 등에 의해 노출 위험이 있다.

권장 수정:
- c4-core의 `secrets.db` (AES-256-GCM)와 통합하거나,
  최소한 `cq hub worker init` 완료 시 "API key is stored in plaintext at ~/.c5/config.yaml" 경고 출력 추가.
- 장기: `cq secret set c5.api_key <val>` 경로로 secrets.db에 저장하고 start 시 `cq secret get` 주입.

---

### `--non-interactive` 플래그 동작 — 심각도: LOW

재현 조건:
- `--non-interactive` 없이 `--hub-url` / `--api-key` 플래그만 전달하면 인터랙티브 프롬프트가 뜬다.
- CI 파이프라인에서 `--hub-url`과 `--api-key`를 플래그로 전달하더라도 `--non-interactive`를 빠뜨리면 stdin이 없어 hang 발생 가능.

권장 수정:
- `--hub-url`과 `--api-key` 가 모두 제공된 경우 자동으로 non-interactive 모드 진입.
- 또는 `--hub-url` / `--api-key` 플래그가 있으면 `workerInitNonInteractive`를 true로 설정하는 조기 분기 추가.

---

### `start` 서브커맨드: C5_API_KEY env 주입 — 심각도: LOW

재현 조건:
- `runWorkerStart`에서 `c.Env = append(os.Environ(), "C5_API_KEY="+cfg.APIKey)` 로 주입한다.
- `os.Environ()`에 이미 `C5_API_KEY`가 설정되어 있으면 두 개의 `C5_API_KEY` 항목이 env에 존재한다.
- Go의 `exec.Cmd`는 마지막 항목을 사용하므로 config의 값이 우선하여 결과적으로 올바르게 동작한다.
- 그러나 의도가 명확하지 않아 혼란을 줄 수 있다.

권장 수정:
- `os.Environ()`을 필터링하여 기존 `C5_API_KEY`를 제거한 후 config 값을 추가.
- 또는 주석으로 "config overrides env" 의도를 명시.

---

### `start` 서브커맨드: `--name` 플래그 미전달 — 심각도: LOW

재현 조건:
- `runWorkerStart`에서 `cfg.Name`이 설정되어 있으면 `--name`을 c5 worker에 전달한다.
- 그러나 `cq hub worker init`에서 `name` 필드를 입력받는 프롬프트가 없다.
- `existing.Name`을 보존하는 로직은 있으나, 초기 설정 시 name을 지정할 방법이 없다.

권장 수정:
- `cq hub worker init`에 `--name` 플래그 또는 인터랙티브 프롬프트 추가.

---

### `install` 서브커맨드: systemd 서비스에 C5_API_KEY 미주입 — 심각도: HIGH

재현 조건:
- `runWorkerInstall`에서 생성하는 systemd/launchd 서비스 파일에 `C5_API_KEY` 환경변수가 주입되지 않는다.
- `buildSystemdUnit`은 `ExecStart` 에 c5 binary와 `--server` 인자만 포함한다.
- 서비스 설치 후 `systemctl enable --now cq-worker`를 실행하면 c5 워커가 API key 없이 시작된다.
- Hub가 API key를 필수로 요구하면 인증 실패로 워커 등록이 거부된다.

권장 수정:
- systemd unit의 `[Service]` 섹션에 `Environment="C5_API_KEY=<key>"` 추가.
  (단, unit 파일에 평문 저장 위험 존재 → `EnvironmentFile=` + secrets file 패턴 권장)
- launchd plist에는 `<key>EnvironmentVariables</key>` 딕셔너리로 주입.

---

## worker.go (c5)

### C4_PROJECT_ID env 주입 경로 — 심각도: 정상 동작 확인

코드 경로:
- `executeJob`의 env 주입 순서 (line ~406):
  1. `env := os.Environ()` — 기본 env 상속
  2. `job.Env` 순회: `k=v` 추가
  3. `job.ProjectID != ""` 조건부 `C4_PROJECT_ID=<val>` 마지막에 추가
  4. `C5_CAPABILITY`, `C5_PARAMS`, `C5_RESULT_FILE` 추가

- `C4_PROJECT_ID`가 `job.Env`보다 나중에 추가되므로 job payload의 project_id가 최종 우선권을 가진다.
- Stateless Worker 문서의 "Hub가 잡 payload에 project_id 포함 → 워커가 자식 프로세스에 C4_PROJECT_ID env 주입"과 일치.

잠재적 이슈:
- `job.Env`에 `C4_PROJECT_ID`가 포함되면 덮어씌워진다. Go exec 마지막 값 우선 규칙상 실제로는 명시적 주입이 이긴다. 하지만 job.Env의 C4_PROJECT_ID를 먼저 필터링하지 않아 env에 중복 항목이 남는다.

---

### control:upgrade 수신 → cq upgrade 실행 경로 — 심각도: 정상 동작 확인

코드 경로 (line ~260):
```go
case "upgrade":
    cqPath := findCQBinary()
    if cqPath == "" {
        log.Printf("c5-worker: cq upgrade failed: cq binary not found — retrying next poll")
        break
    }
    if err := exec.Command(cqPath, "upgrade").Run(); err != nil {
        log.Printf("c5-worker: cq upgrade failed: %v — retrying next poll")
    } else {
        os.Exit(0)
    }
```

잠재적 이슈:
- `findCQBinary`가 `~/.local/bin/cq`까지 확인하므로 daemon PATH 문제는 처리됨.
- `cq upgrade` 성공 후 `os.Exit(0)`으로 종료 → 재시작은 systemd `Restart=on-failure`에 의존.
- systemd unit의 `Restart=on-failure`는 exit code 0에 반응하지 않는다.
  즉, upgrade 후 워커가 자동 재시작되지 않을 수 있다.

권장 수정:
- `Restart=always` 또는 `os.Exit(1)` (의도적 비정상 종료로 restart 트리거)로 변경.
  또는 unit 파일에 `SuccessExitStatus=0`과 `RestartForceExitStatus=0` 추가.

---

## sqlite.go (c5/internal/store)

### UptimeSec, LastJobAt 조건부 업데이트 — 심각도: 정상 동작 확인

`UpdateHeartbeat` (line ~661) 동작:
```go
if req.UptimeSec > 0 {
    query += ", uptime_sec = ?"
    args = append(args, req.UptimeSec)
}
if req.LastJobAt != "" {
    query += ", last_job_at = ?"
    args = append(args, req.LastJobAt)
}
```

- `UptimeSec > 0` 조건: 0을 "미설정"으로 해석하여 업데이트를 건너뛴다.
  worker 시작 직후(uptime=0)에는 uptime_sec 컬럼이 업데이트되지 않는다. 의도된 동작으로 보임.
- `LastJobAt != ""` 조건: 잡이 없을 때 빈 문자열이 기존 last_job_at을 덮어쓰지 않는다. 올바른 설계.

잠재적 이슈:
- worker 재시작 후 uptime_sec가 리셋(0 →다음 heartbeat까지 0)되는 동안 UI가 0을 표시할 수 있다.
  큰 문제는 아니나 UX 관점에서 "just restarted" 상태임을 알기 어렵다.

---

### workers 테이블 스키마: name/uptime_sec/last_job_at 마이그레이션 — 심각도: 정상 동작 확인

초기 CREATE TABLE (line ~90)에는 `name`, `uptime_sec`, `last_job_at` 컬럼이 없다.
해당 컬럼들은 `ALTER TABLE workers ADD COLUMN` 마이그레이션으로 추가된다 (line ~333-336).

마이그레이션 패턴은 "duplicate column" 에러를 무시하는 방식으로 idempotent하다. 정상.

---

## 핵심 UX 마찰점

### CI 변수명 불일치 — 심각도: HIGH

문제:
- GitLab CI/CD 변수 컨벤션: `CI_HUB_URL`, `CI_API_KEY` (CI_ prefix는 GitLab 예약 네임스페이스)
- c5 워커가 기대하는 변수: `C5_HUB_URL`, `C5_API_KEY`
- hub_worker.go `runWorkerInit`에서 `--hub-url`/`--api-key` 플래그 또는 stdin만 읽으며, env 변수 fallback이 없다.
- `c5/cmd/c5/worker.go`의 `workerCmd()`는 `C5_HUB_URL`과 `C5_API_KEY`를 env에서 읽는다.

재현 조건:
```bash
# GitLab CI .gitlab-ci.yml 예시
variables:
  CI_HUB_URL: "https://hub.example.com"
  CI_API_KEY: "secret"

deploy:
  script:
    - cq hub worker init --non-interactive --hub-url "$CI_HUB_URL" --api-key "$CI_API_KEY"
    # ↑ 작동은 하지만 사용자가 직접 변수명 매핑을 해야 한다.
    # c5 직접 실행 시: c5 worker --server $C5_HUB_URL
    # 하지만 환경에는 CI_HUB_URL이 있고 C5_HUB_URL이 없어 fallback이 동작하지 않는다.
```

권장 수정 옵션 A: `runWorkerInit`에 env fallback 추가
```go
// hub-url flag가 없으면 C5_HUB_URL → CI_HUB_URL 순서로 fallback
if workerInitHubURL == "" {
    if v := os.Getenv("C5_HUB_URL"); v != "" {
        workerInitHubURL = v
    } else if v := os.Getenv("CI_HUB_URL"); v != "" {
        workerInitHubURL = v
    }
}
// api-key도 동일하게 C5_API_KEY → CI_API_KEY fallback
```

권장 수정 옵션 B: `c5/cmd/c5/worker.go`의 workerCmd에 CI_* fallback 추가
```go
defaultServer = os.Getenv("C5_HUB_URL")
if defaultServer == "" {
    defaultServer = os.Getenv("CI_HUB_URL") // GitLab CI 호환
}
```

---

### install 단계 통합 — 심각도: HIGH

문제:
현재 GPU 워커 온보딩은 3단계 분리:
1. `curl install.sh | bash` — cq + c5 바이너리 설치만 수행, 워커 설정 없음
2. `cq hub worker init [--non-interactive ...]` — 별도 명령으로 credentials 설정
3. `cq hub worker start` 또는 `cq hub worker install` — 워커 시작

CI/CD 파이프라인에서의 이상적 UX:
```bash
curl -sSL https://install.example.com | CI_HUB_URL=https://hub.example.com CI_API_KEY=secret bash
# → cq + c5 설치 + worker init 자동 수행 → 즉시 start 가능
```

그러나 현재 `install.sh`는 `cq doctor`만 실행하고 worker init을 수행하지 않는다.

재현 조건:
- `CI_HUB_URL`과 `CI_API_KEY` (또는 `C5_HUB_URL`, `C5_API_KEY`)가 환경에 있을 때
  install.sh가 자동으로 `cq hub worker init --non-interactive`를 실행하지 않는다.
- 사용자가 이 단계를 수동으로 추가해야 하며, README를 놓치면 워커가 동작하지 않는다.

권장 수정:
- `install.sh` 후반부에 조건부 auto-init 추가:
```bash
# Auto-init worker if hub credentials provided
if [ -n "${C5_HUB_URL:-$CI_HUB_URL}" ] && [ -n "${C5_API_KEY:-$CI_API_KEY}" ]; then
  HUB_URL="${C5_HUB_URL:-$CI_HUB_URL}"
  API_KEY="${C5_API_KEY:-$CI_API_KEY}"
  echo "Auto-configuring worker credentials..."
  "${INSTALL_DIR}/cq" hub worker init --non-interactive \
    --hub-url "$HUB_URL" --api-key "$API_KEY" || \
    echo "Warning: worker init failed — run manually: cq hub worker init"
fi
```

---

## 요약

| 이슈 | 파일 | 심각도 |
|------|------|--------|
| systemd 서비스에 C5_API_KEY 미주입 | hub_worker.go | HIGH |
| CI 변수명 불일치 (CI_* vs C5_*) | hub_worker.go / worker.go | HIGH |
| install.sh 단계 미통합 | user/install.sh | HIGH |
| API key 평문 저장 (~/.c5/config.yaml) | hub_worker.go | MEDIUM |
| control:upgrade 후 systemd 재시작 미발동 | worker.go | MEDIUM |
| --non-interactive 없이 플래그 전달 시 hang | hub_worker.go | LOW |
| C5_API_KEY 중복 env 항목 | hub_worker.go | LOW |
| init에서 --name 설정 불가 | hub_worker.go | LOW |
