---
name: db-design
description: |
  데이터베이스 스키마 설계 가이드. 테이블 구조, 인덱스, 관계, 정규화, 성능 고려사항까지
  체계적으로 설계합니다. 새 테이블이나 스키마 변경이 필요할 때 반드시 이 스킬을 사용하세요.
  "DB 설계", "db design", "테이블 설계", "스키마 설계", "인덱스 설계",
  "ERD", "데이터 모델링" 등의 요청에 트리거됩니다.
---

# DB Design

데이터베이스 스키마 설계 가이드.

## 트리거

"DB 설계", "db design", "테이블 설계", "스키마 설계", "인덱스 설계", "ERD"

## Steps

### 1. 도메인 모델링

- 엔티티(명사) 식별: User, Order, Product, ...
- 관계 정의: 1:1, 1:N, N:M
- 속성 나열: 각 엔티티의 필드
- 비즈니스 규칙 확인: "주문은 반드시 사용자가 있어야 한다" 등

### 2. 테이블 설계

```sql
CREATE TABLE users (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email       TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**규칙:**
- PK: `id` (BIGINT 권장, UUID는 인덱스 성능 고려)
- 타임스탬프: `created_at`, `updated_at` 필수 (TIMESTAMPTZ)
- NOT NULL 기본: nullable은 명시적 이유가 있을 때만
- TEXT > VARCHAR: PostgreSQL에서 성능 차이 없음
- ENUM 대신 참조 테이블 또는 CHECK constraint

### 3. 관계 설계

```sql
-- 1:N (FK는 N쪽에)
ALTER TABLE orders ADD CONSTRAINT fk_user
    FOREIGN KEY (user_id) REFERENCES users(id);

-- N:M (조인 테이블)
CREATE TABLE order_items (
    order_id    BIGINT REFERENCES orders(id),
    product_id  BIGINT REFERENCES products(id),
    quantity    INT NOT NULL DEFAULT 1,
    PRIMARY KEY (order_id, product_id)
);
```

**FK 규칙:**
- CASCADE DELETE: 부모 삭제 시 자식도 삭제 (로그 제외)
- RESTRICT: 참조 무결성 강제 (기본값)
- SET NULL: 부모 삭제 시 FK null (소프트 삭제용)

### 4. 인덱스 설계

```sql
-- 자주 조회되는 컬럼
CREATE INDEX idx_orders_user_id ON orders(user_id);

-- 복합 인덱스 (순서 중요: 카디널리티 높은 것 먼저)
CREATE INDEX idx_orders_status_created ON orders(status, created_at DESC);

-- 부분 인덱스 (조건부)
CREATE INDEX idx_orders_pending ON orders(created_at) WHERE status = 'pending';
```

**원칙:**
- WHERE, JOIN, ORDER BY에 사용되는 컬럼
- 쓰기 빈번한 테이블: 인덱스 최소화 (INSERT 성능)
- EXPLAIN ANALYZE로 실행 계획 확인 후 추가
- 사용 안 되는 인덱스 주기적 정리

### 5. 정규화 vs 비정규화

| | 정규화 | 비정규화 |
|--|--------|---------|
| 장점 | 데이터 정합성, 저장 공간 | 조회 성능 |
| 단점 | JOIN 비용 | 갱신 이상, 중복 |
| 적용 | OLTP (일반적) | OLAP, 읽기 집중 |

**3NF까지 정규화 후, 측정된 성능 문제가 있을 때만 비정규화.**

### 6. 체크리스트

- [ ] 모든 테이블에 PK가 있는가?
- [ ] FK 제약 조건이 설정되어 있는가?
- [ ] NOT NULL이 적절히 설정되어 있는가?
- [ ] 인덱스가 주요 쿼리 패턴을 커버하는가?
- [ ] 마이그레이션 스크립트가 작성되어 있는가? (up + down)
- [ ] 대용량 테이블에 파티셔닝이 고려되었는가?

## 안티패턴

- 모든 컬럼 VARCHAR(255) (의미 없는 길이 제한)
- 인덱스 없이 "느려지면 그때 추가" (설계 시 함께)
- 소프트 삭제(`deleted_at`) 남용 (쿼리 복잡도 증가)
- EAV(Entity-Attribute-Value) 패턴 (JSONB가 낫다)
