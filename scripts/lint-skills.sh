#!/usr/bin/env bash
# lint-skills.sh — Skill 계약 검증
# Runs Go arch tests that enforce:
#   1. Deprecated skills are stubs (≤ 20 lines)
#   2. Finish skills have knowledge gate (c4_knowledge_record / c4_experiment_record)
#   3. Plan skills have knowledge read gate (c4_knowledge_search / c4_pattern_suggest)
set -e
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT/c4-core"
exec go test ./test/archtest/ -v -run "TestDeprecated|TestFinish|TestPlan" "$@"
