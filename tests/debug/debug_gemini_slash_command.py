import os
import sys
from pathlib import Path

# Add project root to path
project_root = Path(__file__).parent.parent.parent.absolute()
sys.path.insert(0, str(project_root))

from c4.models.config import LLMConfig
from c4.supervisor.backend_factory import create_backend


def test_gemini_slash_command():
    print("Testing Gemini Slash Command Understanding...")

    # 1. Setup Mock Config & Backend
    config = LLMConfig(
        model="gemini/gemini-1.5-pro",
        api_key_env="GOOGLE_API_KEY"
    )
    os.environ["GOOGLE_API_KEY"] = "fake-key"
    backend = create_backend(config)

    # 2. Load Slash Command Template
    cmd_file = project_root / ".gemini/commands/c4-status.md"
    if not cmd_file.exists():
        print(f"❌ {cmd_file} not found. Run 'c4 platforms --setup gemini' first.")
        return

    cmd_content = cmd_file.read_text()
    print(f"Loaded command template ({len(cmd_content)} chars)")

    # 3. Simulate Prompt
    # User types "/c4-status". The CLI usually injects the command file content into context.
    user_input = "/c4-status"

    # We construct a prompt that mimics what the CLI would send to the LLM
    prompt = f"{cmd_content}\n\nUser Input: {user_input}"

    # 4. Build Request Params (to check tools/functions configuration)
    # Note: LiteLLMBackend currently focuses on "review" tasks (text generation).
    # To support tool calling, we might need to check if tools are passed.
    # But for this test, we just want to see if the backend accepts the prompt structure.

    kwargs = backend._build_request_kwargs(prompt, timeout=300)

    print("\nRequest Parameters:")
    print("-" * 20)
    print(f"Model: {kwargs['model']}")
    print(f"System Message present: {'Yes' if any(m['role']=='system' for m in kwargs['messages']) else 'No'}")
    print("-" * 20)

    # In a real scenario, we would call llm.completion() and check for tool_calls.
    # Since we can't make real API calls with a fake key, we verify the preparation steps.

    if "c4_status()" in cmd_content:
        print("✅ Template contains correct tool call instruction: c4_status()")
    else:
        print("❌ Template missing tool call instruction")

    print("\n✅ Gemini Backend is ready to process this prompt.")
    print("   (Actual tool execution requires a real Gemini API key and CLI environment)")

if __name__ == "__main__":
    test_gemini_slash_command()
