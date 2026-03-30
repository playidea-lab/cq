# CQ Drive — 기기 간 파일 공유

> USB, 이메일, 별도 클라우드 스토리지 설정 없이 기기와 팀원 간에 파일을 전송한다.

---

## 1. 업로드

파일 하나, 폴더, 또는 여러 파일을 한 번에 업로드한다.

```bash
cq drive upload model.pt                    # 단일 파일
cq drive upload ./results/ --as exp-001     # 폴더
cq drive upload *.csv --to datasets/        # 여러 파일
```

CQ가 경로가 파일인지 폴더인지 자동으로 감지한다. 지원 플래그:

| 플래그 | 설명 |
|--------|------|
| `--to <path>` | 스토리지 내 저장 경로 |
| `--as <name>` | 업로드 시 파일/폴더 이름 변경 |

---

## 2. 다운로드

CQ Drive에서 로컬 기기로 파일을 다운로드한다.

```bash
cq drive download models/best.pt
cq drive download models/best.pt -o ./local/
```

| 플래그 | 설명 |
|--------|------|
| `-o <dir>` | 저장 디렉토리 (기본값: 현재 디렉토리) |

---

## 3. 공유

누구나 다운로드할 수 있는 프리사인드 URL을 생성한다 — 계정 불필요.

```bash
cq drive share checkpoints/model.pt              # 기본 1시간
cq drive share checkpoints/model.pt --ttl 24h    # 24시간
# → https://...supabase.co/storage/v1/object/sign/...?token=xxx
```

- 기본 TTL: **1시간**
- 최대 TTL: **7일**
- CQ 계정 없이도 링크만 있으면 다운로드 가능.

---

## 4. 데이터셋

콘텐츠 해시 중복 제거를 지원하는 버전 관리 데이터셋.

```bash
cq drive dataset upload ./scan_data --as dental-v3
cq drive dataset list
cq drive dataset list dental-v3          # 특정 데이터셋의 버전 목록 조회
cq drive dataset pull dental-v3          # 최신 버전 다운로드
```

CQ는 SHA256 콘텐츠 해시로 버전 간 중복 파일을 제거한다 — 변경된 파일만 업로드된다. 업로드 후 `.cqdata` 파일이 로컬에 생성되어 git에서 데이터셋 버전을 추적한다.

---

## 5. 데이터셋 동기화

`git pull` 이후 데이터셋을 자동으로 동기화한다.

```bash
cq dataset sync              # 변경된 데이터셋만 pull
cq dataset sync --dry-run    # 동기화될 항목 미리보기
```

저장소에서 `cq init`을 한 번 실행하면 `post-merge` git 훅이 설치된다. 워크플로우:

1. 기기 A에서 데이터셋 업로드 → `.cqdata` 파일 업데이트
2. `.cqdata` 파일을 `git push`
3. 기기 B에서 `git pull` → 훅이 `cq dataset sync` 자동 실행
4. 콘텐츠 해시가 변경된 데이터셋만 다운로드

---

## 6. 안정적인 전송

대용량 파일 전송은 재시작 가능하며 네트워크 중단에도 견딘다.

- **TUS 재개 가능 업로드**: 10MB 이상 파일에 적용 (6MB 청크)
- **범위 기반 다운로드 재개**: `.part` 파일로 중단 지점부터 재개
- WSL2 NAT 끊김, 불안정한 Wi-Fi, VPN 재연결에도 중단 없이 동작

별도 플래그 불필요 — 파일 크기에 따라 자동으로 적용된다.

---

## 7. MCP 도구

Claude Code 내에서 작업할 때는 CLI 대신 MCP 도구를 직접 사용한다:

| 도구 | 설명 |
|------|------|
| `cq_drive_upload` | CQ Drive에 파일 업로드 |
| `cq_drive_download` | CQ Drive에서 파일 다운로드 |
| `cq_drive_share` | 프리사인드 다운로드 URL 생성 |
| `cq_drive_list` | 스토리지 파일 목록 조회 |
| `cq_drive_dataset_upload` | 디렉토리를 버전 관리 데이터셋으로 업로드 |
| `cq_drive_dataset_pull` | 이름으로 데이터셋 다운로드 |
| `cq_drive_dataset_list` | 데이터셋 및 버전 목록 조회 |

---

## 8. 활용 시나리오

**GPU 서버와 노트북 간 모델 전송**

```bash
# GPU 서버에서
cq drive upload ./checkpoints/epoch_100.pt --to models/

# 노트북에서
cq drive download models/epoch_100.pt
```

**팀원에게 결과 공유 (계정 불필요)**

```bash
cq drive share results/exp042.json --ttl 24h
# Slack, 이메일 등 어디서든 링크 전송
```

**여러 기기 간 데이터셋 동기화 유지**

```bash
# 각 기기에서 최초 1회만 실행
cq init   # post-merge git 훅 설치

# 데이터셋 업로드 후 .cqdata 파일 커밋
cq drive dataset upload ./data/processed --as dental-v3
git add .cqdata && git commit -m "chore: update dental-v3 dataset ref"
git push

# 다른 기기에서 — git pull 후 자동 동기화
git pull
# 훅 실행: cq dataset sync
```

**실패한 대용량 파일 전송 재개**

```bash
# 업로드 시작 — 네트워크 장애로 중단
cq drive upload ./big_model_10gb.pt

# 동일 명령어 재실행 — 마지막 청크부터 재개
cq drive upload ./big_model_10gb.pt
```
