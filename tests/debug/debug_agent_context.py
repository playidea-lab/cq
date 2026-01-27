import os
import sys
from pathlib import Path

# Add project root to path
project_root = Path(__file__).parent.parent.parent.absolute()
sys.path.insert(0, str(project_root))

# Set C4_PROJECT_ROOT for ContextLoader
os.environ["C4_PROJECT_ROOT"] = str(project_root)

from c4.models.config import LLMConfig
from c4.supervisor.backend_factory import create_backend
from c4.supervisor.context_loader import ContextLoader
from c4.supervisor.agent_graph.loader import AgentGraphLoader
from c4.supervisor.agent_graph.graph import AgentGraph

def test_agent_context():
    print("Testing Agent Context & Standards Injection...")
    
    # 1. Setup Mock Config
    config = LLMConfig(
        model="gemini/gemini-1.5-pro",
        api_key_env="GOOGLE_API_KEY"
    )
    os.environ["GOOGLE_API_KEY"] = "fake-key"
    
    # 2. Create Backend
    backend = create_backend(config)
    print(f"Backend: {backend.name}")

    # 3. Load Standards (Verification 1)
    standards = ContextLoader.load_standards(project_root=project_root)
    if standards and "# PROJECT STANDARDS & RULES" in standards:
        print("✅ Standards loaded successfully")
        print(f"   Length: {len(standards)} chars")
    else:
        print("❌ Standards loading failed")

    # 4. Load Agent Persona (Verification 2)
    # Simulate loading the PIQ domain agent
    try:
        loader = AgentGraphLoader(base_dir=project_root / "c4/supervisor/agent_graph/examples")
        agents = loader.load_agents()
        piq_agent = next((a for a in agents if "piq" in a.id), None)
        
        if piq_agent:
            print(f"✅ Found PIQ Agent: {piq_agent.name}")
            print(f"   Persona Preview: {piq_agent.persona[:100]}...")
        else:
            print("⚠️ PIQ Agent not found in examples (using generic)")
            
    except Exception as e:
        print(f"❌ Error loading agents: {e}")

    # 5. Simulate Request Construction
    # Currently, LiteLLMBackend uses a hardcoded system message + standards.
    # We want to verify if we can inject the Agent Persona.
    
    kwargs = backend._build_request_kwargs("Test Task", timeout=300)
    system_msg = next((m["content"] for m in kwargs["messages"] if m["role"] == "system"), "")
    
    print("\nSystem Message Analysis:")
    if "# PROJECT STANDARDS & RULES" in system_msg:
        print("✅ Standards injected into System Message")
    else:
        print("❌ Standards NOT in System Message")

    if "You are a code review supervisor" in system_msg:
        print("ℹ️ Uses Default Supervisor Persona (Hardcoded in Backend)")
    else:
        print("❓ Custom Persona used")

if __name__ == "__main__":
    test_agent_context()
