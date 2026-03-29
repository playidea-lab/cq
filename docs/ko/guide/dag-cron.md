# DAG & Cron 가이드

Hub에서 반복 작업을 예약하고 다단계 파이프라인을 오케스트레이션합니다.

---

## Cron

표준 cron 표현식으로 명령어를 예약 실행합니다.

### Cron 작업 생성

```sh
# MCP
c4_cron_create(name="daily-train", cron_expr="0 9 * * *", command="python train.py")

# CLI
cq hub cron create --name daily-train --expr "0 9 * * *" --command "python train.py"
```

Cron 표현식은 5필드 형식:

```
┌─── 분 (0-59)
│  ┌── 시 (0-23)
│  │  ┌─ 일 (1-31)
│  │  │  ┌ 월 (1-12)
│  │  │  │  ┌ 요일 (0-6, 일=0)
│  │  │  │  │
0  9  *  *  *   → 매일 오전 9시
*/5 * * * *     → 5분마다
0 0 * * 1       → 매주 월요일 자정
```

### 관리

```sh
c4_cron_list()                         # 목록 조회
c4_cron_delete(name="daily-train")     # 삭제
```

`cq serve`가 매 분 cron 테이블을 확인하고, 스케줄이 일치하면 Hub 작업을 자동 제출합니다.

---

## DAG (Directed Acyclic Graph)

노드가 의존 관계 순서대로 실행되는 다단계 파이프라인입니다.

### DAG 생성 및 실행

```sh
# 1. DAG 생성
dag_id = c4_hub_dag_create(name="ml-pipeline")

# 2. 노드 추가
c4_hub_dag_add_node(dag_id=dag_id, node_id="preprocess", command="python preprocess.py")
c4_hub_dag_add_node(dag_id=dag_id, node_id="train",      command="python train.py")
c4_hub_dag_add_node(dag_id=dag_id, node_id="evaluate",   command="python eval.py")

# 3. 의존 관계 선언
c4_hub_dag_add_dep(dag_id=dag_id, from_node="preprocess", to_node="train")
c4_hub_dag_add_dep(dag_id=dag_id, from_node="train",      to_node="evaluate")

# 4. 실행
c4_hub_dag_execute(dag_id=dag_id)
```

결과 파이프라인:

```
preprocess → train → evaluate
```

### 자동 진행

노드가 완료되면 Hub의 `advance_dag`가 하류 노드의 의존성을 확인하고 자동 제출합니다. 폴링 불필요.

### 병렬 노드

의존 관계가 없는 노드는 동시 실행:

```
fetch-data → train-model-a ─┐
           → train-model-b ─┴→ compare
```

---

## 레퍼런스

| 도구 | 설명 |
|------|------|
| `c4_cron_create(name, cron_expr, command)` | Cron 작업 생성 |
| `c4_cron_list()` | 목록 조회 |
| `c4_cron_delete(name)` | 삭제 |
| `c4_hub_dag_create(name)` | DAG 생성 |
| `c4_hub_dag_add_node(dag_id, node_id, command)` | 노드 추가 |
| `c4_hub_dag_add_dep(dag_id, from_node, to_node)` | 의존 관계 추가 |
| `c4_hub_dag_execute(dag_id)` | DAG 실행 |
