#!/bin/bash
# c4-alchemy.sh: 프로젝트의 모든 지식을 합성하여 통찰을 추출함

KNOWLEDGE_DIR=".c4/knowledge/docs"
TEMP_ALL_DOCS="/tmp/all_knowledge.md"

echo "🧪 지식 연금술 가동 중... 모든 문서를 융합하고 있습니다."

# 1. 모든 지식 문서를 하나로 합치기 (최근 순)
echo "# Compiled Project Knowledge (As of $(date))" > $TEMP_ALL_DOCS
ls -t $KNOWLEDGE_DIR/*.md | xargs cat >> $TEMP_ALL_DOCS

# 2. Gemini 3.0 헤드리스 호출 (Knowledge Alchemist 지침 적용)
./scripts/gemini-headless.sh "
당신은 지식 연금술사입니다. 아래에 제공된 프로젝트의 모든 실험 기록, 인사이트, 패턴들을 읽고 다음을 수행하세요:
1. 서로 다른 문서 간의 연관성(Synthesis)을 2개 이상 찾으세요.
2. 현재 연구/개발 흐름에서 논리적 모순이나 중복된 시도가 있다면 경고하세요.
3. 가장 가치 있는 '다음 실험/작업'을 구체적으로 제안하세요. (전문 분야에 상관없이 시스템적 통찰 제공)

제공된 데이터:
$(cat $TEMP_ALL_DOCS | head -c 50000) # 일단 안전하게 5만자까지만 (3.0은 더 가능하지만 테스트용)
"

rm $TEMP_ALL_DOCS
