"""Unit tests for C4 Agent Router (Phase 4)"""


from c4.discovery.models import Domain
from c4.models.config import AgentChainDef, AgentConfig
from c4.supervisor.agent_router import (
    DOMAIN_AGENT_MAP,
    TASK_TYPE_AGENT_OVERRIDES,
    AgentChainConfig,
    AgentHandoff,
    AgentRouter,
    build_chain_prompt,
    get_agent_for_task_type,
    get_all_domains,
    get_chain_for_domain,
    get_default_router,
    get_handoff_instructions,
    get_recommended_agent,
    set_default_router,
)


class TestAgentChainConfig:
    """Tests for AgentChainConfig dataclass"""

    def test_basic_creation(self):
        """Test basic AgentChainConfig creation"""
        config = AgentChainConfig(
            primary="frontend-developer",
            chain=["frontend-developer", "test-automator"],
            description="Test config",
        )
        assert config.primary == "frontend-developer"
        assert config.chain == ["frontend-developer", "test-automator"]
        assert config.description == "Test config"
        assert config.handoff_instructions == ""

    def test_primary_added_to_chain_if_missing(self):
        """Test that primary agent is added to chain if not present"""
        config = AgentChainConfig(
            primary="frontend-developer",
            chain=["test-automator", "code-reviewer"],
            description="Test",
        )
        # Primary should be prepended
        assert config.chain[0] == "frontend-developer"
        assert "test-automator" in config.chain
        assert "code-reviewer" in config.chain

    def test_empty_chain_gets_primary(self):
        """Test that empty chain gets primary agent"""
        config = AgentChainConfig(
            primary="general-purpose",
            chain=[],
            description="Test",
        )
        assert config.chain == ["general-purpose"]

    def test_primary_not_duplicated(self):
        """Test that primary is not duplicated if already in chain"""
        config = AgentChainConfig(
            primary="frontend-developer",
            chain=["frontend-developer", "test-automator"],
            description="Test",
        )
        assert config.chain.count("frontend-developer") == 1


class TestDomainAgentMap:
    """Tests for DOMAIN_AGENT_MAP structure"""

    def test_all_domains_have_configs(self):
        """Test that all expected domains have configs"""
        expected_domains = [
            "web-frontend",
            "web-backend",
            "fullstack",
            "ml-dl",
            "mobile-app",
            "infra",
            "library",
            "firmware",
            "unknown",
        ]
        for domain in expected_domains:
            assert domain in DOMAIN_AGENT_MAP, f"Missing domain: {domain}"
            config = DOMAIN_AGENT_MAP[domain]
            assert isinstance(config, AgentChainConfig)
            assert config.primary
            assert config.chain
            assert config.description

    def test_web_frontend_config(self):
        """Test web-frontend domain config"""
        config = DOMAIN_AGENT_MAP["web-frontend"]
        assert config.primary == "frontend-developer"
        assert "frontend-developer" in config.chain
        assert "test-automator" in config.chain
        assert "code-reviewer" in config.chain

    def test_web_backend_config(self):
        """Test web-backend domain config"""
        config = DOMAIN_AGENT_MAP["web-backend"]
        assert config.primary == "backend-architect"
        assert "backend-architect" in config.chain
        assert "python-pro" in config.chain
        assert "test-automator" in config.chain
        assert "code-reviewer" in config.chain

    def test_ml_dl_config(self):
        """Test ml-dl domain config"""
        config = DOMAIN_AGENT_MAP["ml-dl"]
        assert config.primary == "ml-engineer"
        assert "ml-engineer" in config.chain
        assert "python-pro" in config.chain

    def test_unknown_domain_has_general_purpose(self):
        """Test unknown domain uses general-purpose agent"""
        config = DOMAIN_AGENT_MAP["unknown"]
        assert config.primary == "general-purpose"


class TestGetRecommendedAgent:
    """Tests for get_recommended_agent function"""

    def test_with_string_domain(self):
        """Test with string domain input"""
        config = get_recommended_agent("web-frontend")
        assert config.primary == "frontend-developer"

    def test_with_domain_enum(self):
        """Test with Domain enum input"""
        config = get_recommended_agent(Domain.WEB_FRONTEND)
        assert config.primary == "frontend-developer"

    def test_with_none_returns_unknown(self):
        """Test that None returns unknown config"""
        config = get_recommended_agent(None)
        assert config.primary == "general-purpose"

    def test_with_invalid_domain_returns_unknown(self):
        """Test that invalid domain returns unknown config"""
        config = get_recommended_agent("invalid-domain-xyz")
        assert config.primary == "general-purpose"

    def test_case_normalization(self):
        """Test that domain case is normalized"""
        config = get_recommended_agent("WEB-FRONTEND")
        assert config.primary == "frontend-developer"

    def test_underscore_to_hyphen(self):
        """Test that underscores are converted to hyphens"""
        config = get_recommended_agent("web_frontend")
        assert config.primary == "frontend-developer"


class TestGetAgentForTaskType:
    """Tests for get_agent_for_task_type function"""

    def test_debug_task_type(self):
        """Test debug task type returns debugger"""
        agent = get_agent_for_task_type("debug", "web-backend")
        assert agent == "debugger"

    def test_security_task_type(self):
        """Test security task type returns security-auditor"""
        agent = get_agent_for_task_type("security", "web-frontend")
        assert agent == "security-auditor"

    def test_performance_task_type(self):
        """Test performance task type returns performance-engineer"""
        agent = get_agent_for_task_type("performance", "ml-dl")
        assert agent == "performance-engineer"

    def test_refactor_task_type(self):
        """Test refactor task type returns code-refactorer"""
        agent = get_agent_for_task_type("refactor", "library")
        assert agent == "code-refactorer"

    def test_test_task_type(self):
        """Test test task type returns test-automator"""
        agent = get_agent_for_task_type("test", "web-backend")
        assert agent == "test-automator"

    def test_none_task_type_uses_domain(self):
        """Test None task type falls back to domain primary"""
        agent = get_agent_for_task_type(None, "web-frontend")
        assert agent == "frontend-developer"

    def test_unknown_task_type_uses_domain(self):
        """Test unknown task type falls back to domain primary"""
        agent = get_agent_for_task_type("unknown-task-type", "web-backend")
        assert agent == "backend-architect"

    def test_case_normalization(self):
        """Test task type case is normalized"""
        agent = get_agent_for_task_type("DEBUG", "web-backend")
        assert agent == "debugger"


class TestTaskTypeOverrides:
    """Tests for TASK_TYPE_AGENT_OVERRIDES structure"""

    def test_all_overrides_are_valid_agents(self):
        """Test all task type overrides map to reasonable agent names"""
        valid_agents = {
            "debugger",
            "performance-engineer",
            "security-auditor",
            "database-optimizer",
            "api-documenter",
            "code-refactorer",
            "test-automator",
            "deployment-engineer",
            "graphql-architect",
            "payment-integration",
            "data-engineer",
            "data-scientist",
        }
        for task_type, agent in TASK_TYPE_AGENT_OVERRIDES.items():
            assert agent in valid_agents, f"Unknown agent: {agent} for task type: {task_type}"


class TestGetChainForDomain:
    """Tests for get_chain_for_domain function"""

    def test_returns_chain_list(self):
        """Test that chain list is returned"""
        chain = get_chain_for_domain("web-frontend")
        assert isinstance(chain, list)
        assert len(chain) >= 1
        assert "frontend-developer" in chain

    def test_none_domain_returns_unknown_chain(self):
        """Test None domain returns unknown chain"""
        chain = get_chain_for_domain(None)
        assert "general-purpose" in chain


class TestGetHandoffInstructions:
    """Tests for get_handoff_instructions function"""

    def test_returns_string(self):
        """Test that handoff instructions are returned"""
        instructions = get_handoff_instructions("web-frontend")
        assert isinstance(instructions, str)
        assert len(instructions) > 0

    def test_web_backend_has_api_mention(self):
        """Test web-backend instructions mention API"""
        instructions = get_handoff_instructions("web-backend")
        assert "API" in instructions or "api" in instructions.lower()


class TestGetAllDomains:
    """Tests for get_all_domains function"""

    def test_returns_all_domains(self):
        """Test that all domains are returned"""
        domains = get_all_domains()
        assert "web-frontend" in domains
        assert "web-backend" in domains
        assert "ml-dl" in domains
        assert "unknown" in domains


class TestAgentHandoff:
    """Tests for AgentHandoff dataclass"""

    def test_basic_creation(self):
        """Test basic AgentHandoff creation"""
        handoff = AgentHandoff(
            from_agent="frontend-developer",
            to_agent="test-automator",
            summary="Implemented login form component",
        )
        assert handoff.from_agent == "frontend-developer"
        assert handoff.to_agent == "test-automator"
        assert handoff.summary == "Implemented login form component"
        assert handoff.files_modified == []
        assert handoff.next_steps == []
        assert handoff.warnings == []

    def test_with_all_fields(self):
        """Test AgentHandoff with all fields"""
        handoff = AgentHandoff(
            from_agent="backend-architect",
            to_agent="python-pro",
            summary="Designed API endpoints",
            files_modified=["src/api/routes.py", "src/api/models.py"],
            next_steps=["Implement endpoint handlers", "Add validation"],
            warnings=["Rate limiting not yet implemented"],
        )
        assert len(handoff.files_modified) == 2
        assert len(handoff.next_steps) == 2
        assert len(handoff.warnings) == 1

    def test_to_prompt(self):
        """Test to_prompt method generates markdown"""
        handoff = AgentHandoff(
            from_agent="ml-engineer",
            to_agent="test-automator",
            summary="Trained baseline model",
            files_modified=["models/baseline.py"],
            next_steps=["Write unit tests for model"],
            warnings=["GPU required for training"],
        )
        prompt = handoff.to_prompt()

        assert "## Handoff from ml-engineer" in prompt
        assert "**Summary:** Trained baseline model" in prompt
        assert "**Files Modified:**" in prompt
        assert "models/baseline.py" in prompt
        assert "**Your Tasks:**" in prompt
        assert "Write unit tests for model" in prompt
        assert "**⚠️ Notes:**" in prompt
        assert "GPU required for training" in prompt


class TestBuildChainPrompt:
    """Tests for build_chain_prompt function"""

    def test_first_agent_prompt(self):
        """Test prompt for first agent in chain"""
        prompt = build_chain_prompt(
            task_title="Create login form",
            task_dod="Form validates email and password",
            agent_index=0,
            agent_chain=["frontend-developer", "test-automator", "code-reviewer"],
        )
        assert "# Task: Create login form" in prompt
        assert "Form validates email and password" in prompt
        assert "primary implementer" in prompt
        assert "frontend-developer" in prompt.lower() or "Your Role" in prompt

    def test_middle_agent_prompt(self):
        """Test prompt for middle agent in chain"""
        prompt = build_chain_prompt(
            task_title="Create login form",
            task_dod="Form validates email and password",
            agent_index=1,
            agent_chain=["frontend-developer", "test-automator", "code-reviewer"],
        )
        assert "# Task: Create login form" in prompt
        assert "test engineer" in prompt.lower() or "test" in prompt.lower()

    def test_reviewer_agent_prompt(self):
        """Test prompt for code-reviewer agent"""
        prompt = build_chain_prompt(
            task_title="Create login form",
            task_dod="Form validates email and password",
            agent_index=2,
            agent_chain=["frontend-developer", "test-automator", "code-reviewer"],
        )
        assert "# Task: Create login form" in prompt
        assert "code reviewer" in prompt.lower() or "review" in prompt.lower()

    def test_with_handoff(self):
        """Test prompt includes handoff context"""
        handoff = AgentHandoff(
            from_agent="frontend-developer",
            to_agent="test-automator",
            summary="Implemented component",
        )
        prompt = build_chain_prompt(
            task_title="Create login form",
            task_dod="Form validates email and password",
            agent_index=1,
            agent_chain=["frontend-developer", "test-automator"],
            handoff=handoff,
        )
        assert "Handoff from frontend-developer" in prompt
        assert "Implemented component" in prompt

    def test_with_handoff_instructions(self):
        """Test prompt includes domain handoff instructions"""
        prompt = build_chain_prompt(
            task_title="Create API endpoint",
            task_dod="REST endpoint for user creation",
            agent_index=1,
            agent_chain=["backend-architect", "python-pro"],
            handoff_instructions="Pass API specs and validation requirements",
        )
        assert "Pass API specs" in prompt or "Handoff Guidelines" in prompt

    def test_next_steps_for_non_last_agent(self):
        """Test that non-last agents show next agent info"""
        prompt = build_chain_prompt(
            task_title="Create feature",
            task_dod="Feature complete",
            agent_index=0,
            agent_chain=["frontend-developer", "test-automator", "code-reviewer"],
        )
        assert "test-automator" in prompt or "Next Steps" in prompt


class TestIntegration:
    """Integration tests for agent routing"""

    def test_full_routing_flow(self):
        """Test full routing flow from domain to chain prompt"""
        # Get config for domain
        config = get_recommended_agent("web-frontend")
        assert config.primary == "frontend-developer"

        # Build prompt for first agent
        prompt = build_chain_prompt(
            task_title="Add dark mode toggle",
            task_dod="Toggle switches between light and dark themes",
            agent_index=0,
            agent_chain=config.chain,
            handoff_instructions=config.handoff_instructions,
        )
        assert "Add dark mode toggle" in prompt

        # Simulate handoff
        handoff = AgentHandoff(
            from_agent=config.chain[0],
            to_agent=config.chain[1],
            summary="Implemented toggle component with theme context",
            files_modified=["src/components/ThemeToggle.tsx"],
            next_steps=["Write unit tests for toggle behavior"],
        )

        # Build prompt for second agent
        prompt2 = build_chain_prompt(
            task_title="Add dark mode toggle",
            task_dod="Toggle switches between light and dark themes",
            agent_index=1,
            agent_chain=config.chain,
            handoff=handoff,
            handoff_instructions=config.handoff_instructions,
        )
        assert "ThemeToggle.tsx" in prompt2
        assert "Write unit tests" in prompt2


# =============================================================================
# AgentRouter Class Tests
# =============================================================================


class TestAgentRouter:
    """Tests for AgentRouter class"""

    def test_default_router_uses_builtin_defaults(self):
        """Test router without config uses built-in defaults"""
        router = AgentRouter()
        config = router.get_recommended_agent("web-frontend")
        assert config.primary == "frontend-developer"

    def test_router_with_custom_domain(self):
        """Test router with custom domain configuration"""
        custom_config = AgentConfig(
            chains={
                "my-custom-domain": AgentChainDef(
                    primary="custom-agent",
                    chain=["custom-agent", "reviewer"],
                    handoff="Custom handoff instructions",
                )
            }
        )
        router = AgentRouter(config=custom_config)

        # Custom domain should work
        config = router.get_recommended_agent("my-custom-domain")
        assert config.primary == "custom-agent"
        assert config.chain == ["custom-agent", "reviewer"]
        assert config.handoff_instructions == "Custom handoff instructions"

        # Built-in defaults should still work
        config = router.get_recommended_agent("web-frontend")
        assert config.primary == "frontend-developer"

    def test_router_override_builtin_domain(self):
        """Test that custom config can override built-in domain"""
        custom_config = AgentConfig(
            chains={
                "web-frontend": AgentChainDef(
                    primary="my-frontend-agent",
                    chain=["my-frontend-agent", "my-tester"],
                    handoff="My custom handoff",
                )
            }
        )
        router = AgentRouter(config=custom_config)

        config = router.get_recommended_agent("web-frontend")
        assert config.primary == "my-frontend-agent"
        assert "my-tester" in config.chain

    def test_router_custom_task_overrides(self):
        """Test router with custom task type overrides"""
        custom_config = AgentConfig(
            task_overrides={
                "my-task-type": "my-special-agent",
                "debug": "my-debugger",  # Override built-in
            }
        )
        router = AgentRouter(config=custom_config)

        # Custom task type
        agent = router.get_agent_for_task_type("my-task-type")
        assert agent == "my-special-agent"

        # Overridden built-in
        agent = router.get_agent_for_task_type("debug")
        assert agent == "my-debugger"

        # Non-overridden built-in still works
        agent = router.get_agent_for_task_type("security")
        assert agent == "security-auditor"

    def test_router_fallback_domain(self):
        """Test custom fallback domain"""
        custom_config = AgentConfig(
            chains={
                "my-fallback": AgentChainDef(
                    primary="fallback-agent",
                    chain=["fallback-agent"],
                )
            },
            defaults={
                "fallback_domain": "my-fallback",
                "fallback_agent": "fallback-agent",
            },
        )
        router = AgentRouter(config=custom_config)

        # Unknown domain should use custom fallback
        config = router.get_recommended_agent("nonexistent-domain")
        assert config.primary == "fallback-agent"

    def test_router_get_all_domains_includes_custom(self):
        """Test get_all_domains includes custom domains"""
        custom_config = AgentConfig(
            chains={
                "custom-domain-1": AgentChainDef(primary="agent1"),
                "custom-domain-2": AgentChainDef(primary="agent2"),
            }
        )
        router = AgentRouter(config=custom_config)

        domains = router.get_all_domains()
        assert "custom-domain-1" in domains
        assert "custom-domain-2" in domains
        assert "web-frontend" in domains  # Built-in still present

    def test_router_get_chain_for_domain(self):
        """Test get_chain_for_domain method"""
        router = AgentRouter()
        chain = router.get_chain_for_domain("web-frontend")
        assert isinstance(chain, list)
        assert "frontend-developer" in chain

    def test_router_get_handoff_instructions(self):
        """Test get_handoff_instructions method"""
        router = AgentRouter()
        instructions = router.get_handoff_instructions("web-backend")
        assert isinstance(instructions, str)
        assert len(instructions) > 0

    def test_router_primary_added_to_chain(self):
        """Test that primary is added to chain if missing"""
        custom_config = AgentConfig(
            chains={
                "test-domain": AgentChainDef(
                    primary="primary-agent",
                    chain=["other-agent"],  # Primary not in chain
                )
            }
        )
        router = AgentRouter(config=custom_config)

        config = router.get_recommended_agent("test-domain")
        assert config.chain[0] == "primary-agent"
        assert "other-agent" in config.chain


class TestAgentRouterWithDomainEnum:
    """Test AgentRouter with Domain enum"""

    def test_with_domain_enum(self):
        """Test router accepts Domain enum"""
        router = AgentRouter()
        config = router.get_recommended_agent(Domain.WEB_FRONTEND)
        assert config.primary == "frontend-developer"

    def test_custom_override_with_enum_lookup(self):
        """Test custom config works when looking up via enum"""
        custom_config = AgentConfig(
            chains={
                "web-frontend": AgentChainDef(
                    primary="custom-frontend",
                    chain=["custom-frontend"],
                )
            }
        )
        router = AgentRouter(config=custom_config)

        # Lookup via enum (which gets converted to "web-frontend")
        config = router.get_recommended_agent(Domain.WEB_FRONTEND)
        assert config.primary == "custom-frontend"


class TestDefaultRouterFunctions:
    """Test get_default_router and set_default_router"""

    def test_get_default_router(self):
        """Test get_default_router returns router instance"""
        router = get_default_router()
        assert isinstance(router, AgentRouter)

    def test_set_default_router(self):
        """Test set_default_router changes default"""
        custom_config = AgentConfig(
            chains={
                "test-domain": AgentChainDef(primary="test-agent")
            }
        )
        custom_router = AgentRouter(config=custom_config)

        # Save original for cleanup
        original = get_default_router()

        try:
            set_default_router(custom_router)
            router = get_default_router()
            assert router is custom_router
        finally:
            # Restore original
            set_default_router(original)


class TestAgentConfigModels:
    """Test AgentConfig and AgentChainDef models"""

    def test_agent_chain_def_defaults(self):
        """Test AgentChainDef default values"""
        chain_def = AgentChainDef(primary="test-agent")
        assert chain_def.primary == "test-agent"
        assert chain_def.chain == []
        assert chain_def.handoff == ""

    def test_agent_chain_def_full(self):
        """Test AgentChainDef with all fields"""
        chain_def = AgentChainDef(
            primary="test-agent",
            chain=["test-agent", "reviewer"],
            handoff="Test handoff",
        )
        assert chain_def.primary == "test-agent"
        assert chain_def.chain == ["test-agent", "reviewer"]
        assert chain_def.handoff == "Test handoff"

    def test_agent_config_defaults(self):
        """Test AgentConfig default values"""
        config = AgentConfig()
        assert config.chains == {}
        assert config.task_overrides == {}
        assert config.defaults["fallback_domain"] == "unknown"
        assert config.defaults["fallback_agent"] == "general-purpose"

    def test_agent_config_full(self):
        """Test AgentConfig with all fields"""
        config = AgentConfig(
            chains={
                "test": AgentChainDef(primary="agent1"),
            },
            task_overrides={"task1": "agent2"},
            defaults={"fallback_domain": "test"},
        )
        assert "test" in config.chains
        assert config.task_overrides["task1"] == "agent2"
        assert config.defaults["fallback_domain"] == "test"
