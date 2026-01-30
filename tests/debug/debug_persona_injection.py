import os
import sys
from pathlib import Path

# Add project root to path
project_root = Path(__file__).parent.parent.parent.absolute()
sys.path.insert(0, str(project_root))

from c4.models.config import LLMConfig  # noqa: E402
from c4.supervisor.agent_graph.loader import AgentGraphLoader  # noqa: E402
from c4.supervisor.backend_factory import create_backend  # noqa: E402


def test_persona_injection():
    print("Testing Dynamic Persona Injection...")

    # 1. Setup Mock Config
    config = LLMConfig(
        model="gemini/gemini-1.5-pro",
        api_key_env="GOOGLE_API_KEY"
    )
    os.environ["GOOGLE_API_KEY"] = "fake-key"

    backend = create_backend(config)

    # 2. Load Agent
    try:
        loader = AgentGraphLoader(base_dir=project_root / "c4/supervisor/agent_graph/examples")
        agents = loader.load_agents()
        piq_agent = next((a for a in agents if "piq" in a.agent.id), None) or agents[0]
        print(f"Loaded Agent: {piq_agent.agent.name}")
    except Exception as e:
        print(f"❌ Error loading agent: {e}")
        return

    # 3. Build Params with Agent
    kwargs = backend._build_request_kwargs(
        prompt="Review this code",
        timeout=300,
        agent=piq_agent
    )

    # 4. Verify System Message
    messages = kwargs.get("messages", [])
    system_msg = next((m["content"] for m in messages if m["role"] == "system"), "")

    print("\nSystem Message Content:")
    print("-" * 40)
    print(system_msg)
    print("-" * 40)

    # Checks
    checks = {
        "Persona Name": piq_agent.agent.name in system_msg,
        "Persona Role": piq_agent.agent.persona.role in system_msg,
        "Standards Injection": "# PROJECT STANDARDS & RULES" in system_msg,
        "JSON Instruction": "Always respond with a JSON object" in system_msg
    }

    print("\nVerification Results:")
    all_pass = True
    for name, result in checks.items():
        status = "✅" if result else "❌"
        print(f"{status} {name}")
        if not result:
            all_pass = False

    if all_pass:
        print("\n🎉 SUCCESS: Dynamic Persona Injection Working!")
    else:
        print("\n⚠️ FAILURE: Some checks failed.")

if __name__ == "__main__":
    test_persona_injection()
