import sys
from pathlib import Path

# 프로젝트 루트를 경로에 추가
sys.path.append(str(Path.cwd()))

from c4.research.store import ResearchStore


def test_research_loop():
    print("🚀 [Step 1] 연구 프로젝트 초기화 (ResearchStart)")
    store = ResearchStore(Path(".c4/research"))

    project_id = store.create_project(
        name="Gemini Dogfooding: HITL Efficiency Study",
        paper_path="docs/research/sample_paper.pdf",
        target_score=8.5
    )
    iteration_id = store.create_iteration(project_id)
    print(f"✅ 프로젝트 생성 완료: {project_id}")
    print(f"✅ 첫 번째 반복(Iteration) 시작: {iteration_id}")

    print("\n📝 [Step 2] 연구원 리뷰 결과 기록 (ResearchRecord)")
    store.update_iteration(iteration_id,
        review_score=6.0,
        gaps=[{"type": "experiment", "name": "Compute Bottleneck", "status": "pending"}]
    )
    print("✅ 리뷰 점수(6.0) 및 실험 필요성 기록 완료")

    print("\n🔍 [Step 3] 다음 액션 제안 확인 (ResearchNext)")
    next_action = store.suggest_next(project_id)
    print(f"🤖 에이전트 제안: {next_action['action']} (이유: {next_action['reason']})")

    print("\n🤝 [Step 4] 연구원 승인 및 다음 단계 진행 (ResearchApprove)")
    store.update_project(project_id, status="active")
    store.update_iteration(iteration_id, status="done")
    new_iteration_id = store.create_iteration(project_id)
    print(f"✅ 연구원 승인 완료. 새로운 반복 시작: {new_iteration_id}")

    current = store.get_current_iteration(project_id)
    if current.iteration_num == 2:
        print("\n🏆 [Final] End-to-End 연구 루프 검증 성공!")

if __name__ == "__main__":
    test_research_loop()
