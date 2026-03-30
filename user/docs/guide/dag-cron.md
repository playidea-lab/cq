# DAG & Cron Guide

Schedule recurring jobs and orchestrate multi-step pipelines on the Hub.

---

## Cron

Run a command on a schedule using standard cron expressions.

### Create a cron job

```sh
# MCP
c4_cron_create(name="daily-train", cron_expr="0 9 * * *", command="python train.py")

# CLI (via Hub API)
cq hub cron create --name daily-train --expr "0 9 * * *" --command "python train.py"
```

Cron expressions follow standard 5-field format:

```
┌─── minute (0-59)
│  ┌── hour (0-23)
│  │  ┌─ day of month (1-31)
│  │  │  ┌ month (1-12)
│  │  │  │  ┌ day of week (0-6, Sun=0)
│  │  │  │  │
0  9  *  *  *   → every day at 09:00
*/5 * * * *     → every 5 minutes
0 0 * * 1       → every Monday at midnight
```

### Common examples

```sh
# Every day at 9am — run training
c4_cron_create(name="daily-train", cron_expr="0 9 * * *", command="python train.py")

# Every hour — sync data
c4_cron_create(name="hourly-sync", cron_expr="0 * * * *", command="python sync.py")

# Every Monday at midnight — weekly report
c4_cron_create(name="weekly-report", cron_expr="0 0 * * 1", command="python report.py")

# Every 5 minutes — health check
c4_cron_create(name="health-check", cron_expr="*/5 * * * *", command="python check.py")
```

### List and manage cron jobs

```sh
c4_cron_list()                         # List all cron jobs
c4_cron_delete(name="daily-train")     # Delete a cron job
```

**How it works**: `cq serve` checks the cron table every minute. When a job's schedule matches the current time, a Hub job is submitted automatically with the configured command.

---

## DAG (Directed Acyclic Graph)

Orchestrate multi-step pipelines where nodes execute in dependency order.

### Create and execute a DAG

```sh
# 1. Create DAG
dag_id = c4_hub_dag_create(name="ml-pipeline")

# 2. Add nodes (each node = one Hub job)
c4_hub_dag_add_node(dag_id=dag_id, node_id="preprocess", command="python preprocess.py")
c4_hub_dag_add_node(dag_id=dag_id, node_id="train",      command="python train.py")
c4_hub_dag_add_node(dag_id=dag_id, node_id="evaluate",   command="python eval.py")
c4_hub_dag_add_node(dag_id=dag_id, node_id="upload",     command="python upload.py")

# 3. Declare dependencies
c4_hub_dag_add_dep(dag_id=dag_id, from_node="preprocess", to_node="train")
c4_hub_dag_add_dep(dag_id=dag_id, from_node="train",      to_node="evaluate")
c4_hub_dag_add_dep(dag_id=dag_id, from_node="evaluate",   to_node="upload")

# 4. Execute — submits root nodes immediately
c4_hub_dag_execute(dag_id=dag_id)
```

The resulting pipeline:

```
preprocess → train → evaluate → upload
```

### Automatic advancement

When a node completes, the Hub's `advance_dag` RPC automatically checks if downstream nodes are unblocked (all dependencies complete) and submits them. No polling required.

```
preprocess completes
  → advance_dag checks train's deps
  → train is unblocked → submitted automatically
train completes
  → evaluate submitted
evaluate completes
  → upload submitted
```

### Parallel nodes

Nodes with no dependency between them run in parallel:

```sh
dag_id = c4_hub_dag_create(name="parallel-pipeline")

c4_hub_dag_add_node(dag_id=dag_id, node_id="fetch-data",   command="python fetch.py")
c4_hub_dag_add_node(dag_id=dag_id, node_id="train-model-a", command="python train.py --model a")
c4_hub_dag_add_node(dag_id=dag_id, node_id="train-model-b", command="python train.py --model b")
c4_hub_dag_add_node(dag_id=dag_id, node_id="compare",      command="python compare.py")

c4_hub_dag_add_dep(dag_id=dag_id, from_node="fetch-data",    to_node="train-model-a")
c4_hub_dag_add_dep(dag_id=dag_id, from_node="fetch-data",    to_node="train-model-b")
c4_hub_dag_add_dep(dag_id=dag_id, from_node="train-model-a", to_node="compare")
c4_hub_dag_add_dep(dag_id=dag_id, from_node="train-model-b", to_node="compare")

c4_hub_dag_execute(dag_id=dag_id)
```

Execution order:

```
fetch-data → train-model-a ─┐
           → train-model-b ─┴→ compare
```

`train-model-a` and `train-model-b` run in parallel after `fetch-data` completes.

### Monitor DAG progress

```sh
cq hub status <dag_id>      # Overall DAG status
cq hub list --dag <dag_id>  # List all node jobs
```

---

## CLI Reference

| Command | Description |
|---------|-------------|
| `cq hub cron create` | Create cron job |
| `cq hub cron list` | List cron jobs |
| `cq hub cron delete` | Delete cron job |

## MCP Reference

| Tool | Description |
|------|-------------|
| `c4_cron_create(name, cron_expr, command)` | Create cron job |
| `c4_cron_list()` | List cron jobs |
| `c4_cron_delete(name)` | Delete cron job |
| `c4_hub_dag_create(name)` | Create DAG |
| `c4_hub_dag_add_node(dag_id, node_id, command)` | Add node to DAG |
| `c4_hub_dag_add_dep(dag_id, from_node, to_node)` | Add dependency |
| `c4_hub_dag_execute(dag_id)` | Execute DAG |
