# GitHub Actions Secrets Configuration

이 문서는 C4 배포 파이프라인에 필요한 GitHub Secrets 설정을 안내합니다.

## Required Secrets

### Automatic (No Configuration Needed)

| Secret | Description |
|--------|-------------|
| `GITHUB_TOKEN` | GitHub에서 자동 제공. GHCR 푸시 권한 포함 |

## Optional Secrets

배포 방식에 따라 아래 secrets를 설정하세요.

### SSH 배포 (권장)

원격 서버에 SSH로 접속하여 Docker를 실행하는 방식입니다.

| Secret | Description | Example |
|--------|-------------|---------|
| `DEPLOY_HOST` | 배포 대상 서버 호스트 | `deploy.example.com` |
| `DEPLOY_USER` | SSH 사용자명 | `deploy` |
| `DEPLOY_SSH_KEY` | SSH 개인 키 (전체 내용) | `-----BEGIN OPENSSH...` |

#### SSH 키 생성 방법

```bash
# 1. SSH 키 생성 (배포 전용)
ssh-keygen -t ed25519 -C "github-actions-deploy" -f ~/.ssh/deploy_key

# 2. 공개 키를 서버에 등록
ssh-copy-id -i ~/.ssh/deploy_key.pub deploy@your-server.com

# 3. 개인 키를 GitHub Secret에 등록
cat ~/.ssh/deploy_key
# 출력된 전체 내용을 DEPLOY_SSH_KEY로 설정
```

### Kubernetes 배포

Kubernetes 클러스터에 배포하는 방식입니다.

| Secret | Description |
|--------|-------------|
| `KUBECONFIG` | kubeconfig 파일 내용 (Base64 인코딩) |

#### kubeconfig 설정 방법

```bash
# kubeconfig를 Base64로 인코딩
cat ~/.kube/config | base64 -w 0

# 출력 결과를 KUBECONFIG Secret에 저장
```

### 외부 Docker Registry

GitHub Container Registry 대신 다른 레지스트리 사용 시:

| Secret | Description | Example |
|--------|-------------|---------|
| `DOCKER_REGISTRY` | 레지스트리 URL | `registry.example.com` |
| `DOCKER_USERNAME` | 레지스트리 사용자명 | `admin` |
| `DOCKER_PASSWORD` | 레지스트리 비밀번호 | `***` |

### 코드 커버리지 (선택)

| Secret | Description |
|--------|-------------|
| `CODECOV_TOKEN` | Codecov 업로드 토큰 |

## Environment Variables

GitHub Environments에서 설정하는 변수입니다 (Secrets 아님).

| Variable | Description | Example |
|----------|-------------|---------|
| `DEPLOY_URL` | 배포된 서비스 URL | `https://api.c4.example.com` |

## Secrets 설정 방법

1. GitHub Repository로 이동
2. Settings > Secrets and variables > Actions
3. "New repository secret" 클릭
4. Name과 Value 입력 후 저장

### Environment-specific Secrets

production/staging 환경별로 다른 값을 사용하려면:

1. Settings > Environments
2. 환경 생성 (production, staging)
3. Environment secrets 설정

## 보안 권장사항

### DO

- SSH 키는 배포 전용으로 새로 생성
- 최소 권한 원칙 적용 (배포 사용자는 필요한 권한만)
- 주기적으로 키 순환
- 환경별로 다른 credentials 사용

### DON'T

- 루트 계정으로 배포하지 않음
- 개인 SSH 키를 secrets에 등록하지 않음
- secrets를 로그에 출력하지 않음
- 같은 credentials를 여러 환경에서 재사용하지 않음

## 디버깅

### Secret이 제대로 설정되었는지 확인

```yaml
# workflow에서 확인 (값은 마스킹됨)
- name: Check secrets
  run: |
    if [ -n "${{ secrets.DEPLOY_SSH_KEY }}" ]; then
      echo "DEPLOY_SSH_KEY is set"
    else
      echo "DEPLOY_SSH_KEY is NOT set"
    fi
```

### SSH 연결 테스트

```yaml
- name: Test SSH connection
  run: |
    echo "${{ secrets.DEPLOY_SSH_KEY }}" > deploy_key
    chmod 600 deploy_key
    ssh -i deploy_key -o StrictHostKeyChecking=no \
      ${{ secrets.DEPLOY_USER }}@${{ secrets.DEPLOY_HOST }} "echo 'Connected!'"
```

## 관련 파일

- `.github/workflows/deploy.yml`: 배포 파이프라인
- `.github/workflows/test.yml`: 테스트 파이프라인
- `docker/Dockerfile.api`: API 서버 이미지
