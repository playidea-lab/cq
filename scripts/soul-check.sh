#!/bin/bash
# soul-check.sh (v2): 지능형 소울 진화 트리거

SOUL_DIR=".c4/souls/changmin"
RAW_PATTERNS_FILE="$SOUL_DIR/raw_patterns.json"
LAST_EVOLVE_FILE="$SOUL_DIR/last_evolve_ts"
THRESHOLD=10
MIN_INTERVAL=14400 # 4시간 (초 단위)

# 1. 누적 데이터 확인
if [ ! -f "$RAW_PATTERNS_FILE" ]; then exit 0; fi
COUNT=$(jq '. | length' $RAW_PATTERNS_FILE 2>/dev/null || echo 0)

# 2. 시간 간격 확인
NOW=$(date +%s)
LAST_TS=$(cat $LAST_EVOLVE_FILE 2>/dev/null || echo 0)
ELAPSED=$((NOW - LAST_TS))

# 3. 트리거 판단
if [ "$COUNT" -ge "$THRESHOLD" ] && [ "$ELAPSED" -ge "$MIN_INTERVAL" ]; then
    echo "✨ 오늘의 작업 스타일($COUNT건)을 학습하여 소울을 진화시킵니다..."
    ./scripts/soul-evolve.sh
    date +%s > $LAST_EVOLVE_FILE
else
    # 조용히 종료하거나 가벼운 메시지만 출력
    if [ "$COUNT" -gt 0 ]; then
        echo "💡 새로운 학습 데이터가 누적되고 있습니다. ($COUNT/$THRESHOLD)"
    fi
fi
