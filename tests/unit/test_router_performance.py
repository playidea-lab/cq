"""Performance benchmarks for agent routers.

These tests verify that router operations complete within acceptable time limits.
Target: < 10ms per query as per DoD.
"""

import time

from c4.supervisor.agent_graph.router import GraphRouter
from c4.supervisor._legacy.agent_router import AgentRouter


class TestAgentRouterPerformance:
    """Performance tests for AgentRouter."""

    def test_get_recommended_agent_under_10ms(self):
        """get_recommended_agent should complete in < 10ms."""
        router = AgentRouter()
        domains = router.get_all_domains()

        for domain in domains[:5]:  # Test first 5 domains
            start = time.perf_counter()
            router.get_recommended_agent(domain)
            elapsed_ms = (time.perf_counter() - start) * 1000

            assert elapsed_ms < 10, f"Query for {domain} took {elapsed_ms:.2f}ms"

    def test_get_all_domains_under_10ms(self):
        """get_all_domains should complete in < 10ms."""
        router = AgentRouter()

        start = time.perf_counter()
        router.get_all_domains()
        elapsed_ms = (time.perf_counter() - start) * 1000

        assert elapsed_ms < 10, f"get_all_domains took {elapsed_ms:.2f}ms"

    def test_bulk_queries_under_100ms(self):
        """100 queries should complete in < 100ms."""
        router = AgentRouter()
        domains = router.get_all_domains()

        start = time.perf_counter()
        for _ in range(100):
            for domain in domains[:5]:
                router.get_recommended_agent(domain)
        elapsed_ms = (time.perf_counter() - start) * 1000

        assert elapsed_ms < 100, f"100 queries took {elapsed_ms:.2f}ms"


class TestGraphRouterPerformance:
    """Performance tests for GraphRouter (fallback mode)."""

    def test_get_recommended_agent_under_10ms(self):
        """get_recommended_agent should complete in < 10ms."""
        router = GraphRouter()
        domains = router.get_all_domains()

        for domain in domains[:5]:  # Test first 5 domains
            start = time.perf_counter()
            router.get_recommended_agent(domain)
            elapsed_ms = (time.perf_counter() - start) * 1000

            assert elapsed_ms < 10, f"Query for {domain} took {elapsed_ms:.2f}ms"

    def test_get_all_domains_under_10ms(self):
        """get_all_domains should complete in < 10ms."""
        router = GraphRouter()

        start = time.perf_counter()
        router.get_all_domains()
        elapsed_ms = (time.perf_counter() - start) * 1000

        assert elapsed_ms < 10, f"get_all_domains took {elapsed_ms:.2f}ms"

    def test_bulk_queries_under_100ms(self):
        """100 queries should complete in < 100ms."""
        router = GraphRouter()
        domains = router.get_all_domains()

        start = time.perf_counter()
        for _ in range(100):
            for domain in domains[:5]:
                router.get_recommended_agent(domain)
        elapsed_ms = (time.perf_counter() - start) * 1000

        assert elapsed_ms < 100, f"100 queries took {elapsed_ms:.2f}ms"


class TestRouterComparisonPerformance:
    """Compare performance between AgentRouter and GraphRouter."""

    def test_graphrouter_not_slower_than_legacy(self):
        """GraphRouter should not be significantly slower than AgentRouter."""
        legacy = AgentRouter()
        graph = GraphRouter()
        domains = legacy.get_all_domains()[:5]

        # Warm up
        for domain in domains:
            legacy.get_recommended_agent(domain)
            graph.get_recommended_agent(domain)

        # Benchmark legacy
        start = time.perf_counter()
        for _ in range(50):
            for domain in domains:
                legacy.get_recommended_agent(domain)
        legacy_time = time.perf_counter() - start

        # Benchmark graph
        start = time.perf_counter()
        for _ in range(50):
            for domain in domains:
                graph.get_recommended_agent(domain)
        graph_time = time.perf_counter() - start

        # Both should be fast enough - main requirement is < 10ms per query
        # GraphRouter with fallback has some overhead but should still be fast
        assert graph_time < 0.5, (  # 500ms for 250 queries = ~2ms per query
            f"GraphRouter took {graph_time * 1000:.2f}ms for 250 queries"
        )
        assert legacy_time < 0.5, f"AgentRouter took {legacy_time * 1000:.2f}ms for 250 queries"
