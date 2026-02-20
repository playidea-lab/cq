"""C4 Bridge Server -- JSON-RPC over TCP for Go<->Python communication.

Listens on a TCP port and handles JSON-RPC requests from the Go MCP server.
Uses newline-delimited JSON (no protoc/grpcio dependency).

Protocol
--------
Each request is a newline-delimited JSON object::

    {"method": "FindSymbol", "params": {"name": "MyClass", "file_path": "test.py"}}

Response::

    {"result": {...}, "error": null}

Usage::

    server = BridgeServer(port=50051, project_root=Path("/my/project"))
    port = await server.start()
    # ... server handles requests ...
    await server.stop()
"""

from __future__ import annotations

import asyncio
import json
import logging
import os
from pathlib import Path
from typing import Any, Awaitable, Callable

from c4.bridge.events import (
    C2_DOCUMENT_PARSED,
    C2_TEXT_EXTRACTED,
    KNOWLEDGE_RECORDED,
    KNOWLEDGE_SEARCHED,
    RESEARCH_RECORDED,
    RESEARCH_STARTED,
    SRC_C2,
    SRC_KNOWLEDGE,
    SRC_RESEARCH,
    EventCollector,
)
from c4.daemon.code_ops import CodeOps

logger = logging.getLogger(__name__)

# Type alias for method handlers
MethodHandler = Callable[[dict[str, Any]], Awaitable[dict[str, Any]]]

# ---------------------------------------------------------------------------
# Lazy imports -- resolved at first BridgeServer construction, not at module
# load time, to avoid hard failures when optional deps (pynvml, torch) are
# missing. The module-level names below serve as mock targets in tests:
#   patch("c4.bridge.rpc_server.DocumentStore")
#   patch("c4.bridge.rpc_server.KnowledgeSearcher")
#   patch("c4.bridge.rpc_server.GpuMonitor")
#   patch("c4.bridge.rpc_server.GpuJobScheduler")
# ---------------------------------------------------------------------------


def _import_document_store():
    from c4.knowledge.documents import DocumentStore
    return DocumentStore


def _import_knowledge_searcher():
    from c4.knowledge.search import KnowledgeSearcher
    return KnowledgeSearcher


def _import_knowledge_embedder():
    from c4.knowledge.embeddings import KnowledgeEmbedder
    return KnowledgeEmbedder


def _import_gpu_monitor():
    from c4.gpu.monitor import get_gpu_monitor
    return get_gpu_monitor


def _import_gpu_scheduler():
    from c4.gpu.scheduler import GpuJobScheduler
    return GpuJobScheduler


# Module-level aliases for mock targets (resolved lazily by default,
# but tests can patch these names directly).
DocumentStore: Any = None
KnowledgeSearcher: Any = None
KnowledgeEmbedder: Any = None
GpuMonitor: Any = None  # Actually get_gpu_monitor factory
GpuJobScheduler: Any = None
ResearchStore: Any = None


def _import_research_store():
    from c4.research.store import ResearchStore
    return ResearchStore


def _ensure_imports() -> None:
    """Resolve lazy imports once. Safe to call multiple times."""
    global DocumentStore, KnowledgeSearcher, KnowledgeEmbedder, GpuMonitor, GpuJobScheduler, ResearchStore
    if DocumentStore is None:
        try:
            DocumentStore = _import_document_store()
        except ImportError:
            logger.debug("Failed to import DocumentStore")
    if KnowledgeSearcher is None:
        try:
            KnowledgeSearcher = _import_knowledge_searcher()
        except ImportError:
            logger.debug("Failed to import KnowledgeSearcher")
    if KnowledgeEmbedder is None:
        try:
            KnowledgeEmbedder = _import_knowledge_embedder()
        except ImportError:
            logger.debug("Failed to import KnowledgeEmbedder")
    if GpuMonitor is None:
        try:
            GpuMonitor = _import_gpu_monitor()
        except ImportError:
            logger.debug("Failed to import GpuMonitor")
    if GpuJobScheduler is None:
        try:
            GpuJobScheduler = _import_gpu_scheduler()
        except ImportError:
            logger.debug("Failed to import GpuJobScheduler")
    if ResearchStore is None:
        try:
            ResearchStore = _import_research_store()
        except ImportError:
            logger.debug("Failed to import ResearchStore")


class _DaemonStub:
    """Minimal stub satisfying CodeOps(daemon) interface.

    CodeOps only needs ``daemon.root`` (a Path to the project root).
    """

    def __init__(self, root: Path):
        self.root = root


class BridgeServer:
    """JSON-RPC over TCP server bridging Go MCP server to Python implementations.

    Attributes:
        port: TCP port to listen on (0 = OS-assigned).
        project_root: Project root directory for LSP/Knowledge operations.
        methods: Registry of method name -> async handler.
    """

    def __init__(
        self,
        port: int | None = None,
        project_root: Path | None = None,
    ):
        # Port resolution: explicit arg > env var > default 50051
        if port is not None:
            self.port = port
        else:
            env_port = os.environ.get("C4_BRIDGE_PORT", os.environ.get("C4_GRPC_PORT"))
            self.port = int(env_port) if env_port else 50051

        self.project_root = project_root or Path.cwd()

        # Internal state
        self._server: asyncio.Server | None = None
        self._code_ops = self._create_code_ops()
        self._cached_doc_store: Any = None
        self._cached_research_store: Any = None

        # Resolve lazy imports
        _ensure_imports()

        # Register all methods
        self.methods: dict[str, MethodHandler] = {}
        self.methods["Ping"] = self._handle_ping
        self._register_lsp_methods()
        self._register_knowledge_methods()
        self._register_gpu_methods()
        self._register_onboard_methods()
        self._register_research_methods()
        self._register_c2_methods()

    # ======================================================================
    # Health Check
    # ======================================================================

    async def _handle_ping(self, params: dict[str, Any]) -> dict[str, Any]:
        """Ping -> pong. Used by Go sidecar health check."""
        return {"status": "ok"}

    # ======================================================================
    # Server Lifecycle
    # ======================================================================

    async def start(self) -> int:
        """Start the TCP server. Returns the actual port (useful when port=0)."""
        self._server = await asyncio.start_server(
            self._handle_connection,
            host="127.0.0.1",
            port=self.port,
        )
        addr = self._server.sockets[0].getsockname()
        actual_port = addr[1]
        self.port = actual_port
        logger.info("Bridge server listening on 127.0.0.1:%d", actual_port)
        return actual_port

    async def stop(self) -> None:
        """Gracefully stop the server."""
        if self._server is not None:
            self._server.close()
            await self._server.wait_closed()
            self._server = None
        # Close cached stores
        if self._cached_doc_store is not None:
            self._cached_doc_store.close()
            self._cached_doc_store = None
        if self._cached_research_store is not None:
            self._cached_research_store.close()
            self._cached_research_store = None
        logger.info("Bridge server stopped")

    # ======================================================================
    # Connection Handler
    # ======================================================================

    async def _handle_connection(
        self,
        reader: asyncio.StreamReader,
        writer: asyncio.StreamWriter,
    ) -> None:
        """Handle a single TCP connection (multiple sequential requests)."""
        peer = writer.get_extra_info("peername")
        logger.debug("New connection from %s", peer)
        try:
            while True:
                line = await reader.readline()
                if not line:
                    break  # EOF -- client disconnected

                try:
                    response = await self._process_line(line)
                except Exception as exc:
                    logger.exception("_process_line error (connection kept): %s", exc)
                    response = {"result": None, "error": str(exc)}
                writer.write(json.dumps(response).encode() + b"\n")
                await writer.drain()
        except asyncio.CancelledError:
            pass
        except Exception as exc:
            logger.exception("Connection handler error: %s", exc)
        finally:
            writer.close()
            try:
                await writer.wait_closed()
            except Exception:
                pass

    async def _process_line(self, line: bytes) -> dict[str, Any]:
        """Parse a JSON-RPC request line and dispatch."""
        try:
            request = json.loads(line.decode())
        except (json.JSONDecodeError, UnicodeDecodeError) as exc:
            return {"result": None, "error": f"Invalid JSON: {exc}"}

        method = request.get("method")
        if not method:
            return {"result": None, "error": "Missing 'method' field"}

        params = request.get("params", {})
        result = await self.dispatch(method, params)

        if "error" in result and result["error"] is not None:
            return {"result": None, "error": result["error"]}
        return {"result": result, "error": None}

    async def dispatch(self, method: str, params: dict[str, Any]) -> dict[str, Any]:
        """Dispatch a method call to the registered handler.

        Public so tests can call it directly without TCP.
        """
        handler = self.methods.get(method)
        if handler is None:
            return {"error": f"Unknown method: {method}"}
        return await handler(params)

    # ======================================================================
    # CodeOps Setup
    # ======================================================================

    def _create_code_ops(self) -> CodeOps:
        """Create a CodeOps instance backed by a lightweight daemon stub."""
        stub = _DaemonStub(self.project_root)
        return CodeOps(stub)

    # ======================================================================
    # LSP Methods
    # ======================================================================

    def _register_lsp_methods(self) -> None:
        self.methods["FindSymbol"] = self._handle_find_symbol
        self.methods["GetSymbolsOverview"] = self._handle_get_symbols_overview
        self.methods["ReplaceSymbolBody"] = self._handle_replace_symbol_body
        self.methods["InsertBeforeSymbol"] = self._handle_insert_before_symbol
        self.methods["InsertAfterSymbol"] = self._handle_insert_after_symbol
        self.methods["RenameSymbol"] = self._handle_rename_symbol
        self.methods["FindReferencingSymbols"] = self._handle_find_referencing_symbols

    async def _handle_find_symbol(self, params: dict[str, Any]) -> dict[str, Any]:
        """FindSymbol -> CodeOps.find_symbol()."""
        if not isinstance(params, dict):
            return {"error": "params must be a dict"}
        name = params.get("name")
        if not name or not isinstance(name, str):
            return {"error": "name is required and must be a string"}
        try:
            return self._code_ops.find_symbol(
                name_path_pattern=name,
                relative_path=params.get("file_path", params.get("path", "")),
                include_body=params.get("include_body", False),
                depth=params.get("depth", 0),
            )
        except Exception as exc:
            return {"error": f"FindSymbol failed: {exc}"}

    async def _handle_get_symbols_overview(self, params: dict[str, Any]) -> dict[str, Any]:
        """GetSymbolsOverview -> CodeOps.get_symbols_overview()."""
        if not isinstance(params, dict):
            return {"error": "params must be a dict"}
        path = params.get("file_path", params.get("path", ""))
        if not path or not isinstance(path, str):
            return {"error": "file_path or path is required"}
        try:
            return self._code_ops.get_symbols_overview(
                relative_path=path,
                depth=params.get("depth", 0),
            )
        except Exception as exc:
            return {"error": f"GetSymbolsOverview failed: {exc}"}

    async def _handle_replace_symbol_body(self, params: dict[str, Any]) -> dict[str, Any]:
        """ReplaceSymbolBody -> CodeOps.replace_symbol_body()."""
        try:
            return self._code_ops.replace_symbol_body(
                name_path=params.get("symbol_name", ""),
                file_path=params.get("file_path"),
                new_body=params.get("new_body", ""),
            )
        except Exception as exc:
            return {"error": f"ReplaceSymbolBody failed: {exc}"}

    async def _handle_insert_before_symbol(self, params: dict[str, Any]) -> dict[str, Any]:
        """InsertBeforeSymbol -> CodeOps.insert_before_symbol()."""
        try:
            return self._code_ops.insert_before_symbol(
                name_path=params.get("symbol_name", ""),
                file_path=params.get("file_path"),
                content=params.get("content", ""),
            )
        except Exception as exc:
            return {"error": f"InsertBeforeSymbol failed: {exc}"}

    async def _handle_insert_after_symbol(self, params: dict[str, Any]) -> dict[str, Any]:
        """InsertAfterSymbol -> CodeOps.insert_after_symbol()."""
        try:
            return self._code_ops.insert_after_symbol(
                name_path=params.get("symbol_name", ""),
                file_path=params.get("file_path"),
                content=params.get("content", ""),
            )
        except Exception as exc:
            return {"error": f"InsertAfterSymbol failed: {exc}"}

    async def _handle_rename_symbol(self, params: dict[str, Any]) -> dict[str, Any]:
        """RenameSymbol -> CodeOps.rename_symbol()."""
        try:
            return self._code_ops.rename_symbol(
                name_path=params.get("old_name", ""),
                file_path=params.get("file_path"),
                new_name=params.get("new_name", ""),
            )
        except Exception as exc:
            return {"error": f"RenameSymbol failed: {exc}"}

    async def _handle_find_referencing_symbols(self, params: dict[str, Any]) -> dict[str, Any]:
        """FindReferencingSymbols -> CodeOps.find_referencing_symbols()."""
        try:
            return self._code_ops.find_referencing_symbols(
                name_path=params.get("symbol_name", ""),
                file_path=params.get("file_path"),
            )
        except Exception as exc:
            return {"error": f"FindReferencingSymbols failed: {exc}"}

    # ======================================================================
    # Knowledge Methods
    # ======================================================================

    # DEPRECATED: Go native implementation preferred (fallback only)
    def _register_knowledge_methods(self) -> None:
        self.methods["KnowledgeSearch"] = self._handle_knowledge_search
        self.methods["KnowledgeRecord"] = self._handle_knowledge_record
        self.methods["KnowledgeGet"] = self._handle_knowledge_get

    def _knowledge_base_path(self) -> Path:
        return self.project_root / ".c4" / "knowledge"

    def _get_doc_store(self) -> Any:
        """Get or create a cached DocumentStore instance."""
        if self._cached_doc_store is None:
            self._cached_doc_store = DocumentStore(base_path=self._knowledge_base_path())
        return self._cached_doc_store

    async def _handle_knowledge_search(self, params: dict[str, Any]) -> dict[str, Any]:
        """KnowledgeSearch -> KnowledgeSearcher.search()."""
        query = params.get("query")
        if not query:
            return {"error": "query is required"}

        top_k = params.get("top_k", 10)
        filters = params.get("filters")

        try:
            searcher = KnowledgeSearcher(base_path=self._knowledge_base_path())
            results = searcher.search(query, top_k=top_k, filters=filters)
            ec = EventCollector()
            ec.emit(KNOWLEDGE_SEARCHED, SRC_KNOWLEDGE, {
                "query": query,
                "top_k": top_k,
                "result_count": len(results),
            })
            return ec.attach({"count": len(results), "results": results})
        except Exception as exc:
            return {"error": f"KnowledgeSearch failed: {exc}"}

    async def _handle_knowledge_record(self, params: dict[str, Any]) -> dict[str, Any]:
        """KnowledgeRecord -> DocumentStore.create()."""
        doc_type = params.get("doc_type")
        title = params.get("title")

        if not doc_type:
            return {"error": "doc_type is required"}
        if not title:
            return {"error": "title is required"}

        valid_types = {"experiment", "pattern", "insight", "hypothesis"}
        if doc_type not in valid_types:
            return {"error": f"Invalid doc_type: {doc_type}. Must be one of {valid_types}"}

        body = params.get("body", "")

        _VALID_METADATA_FIELDS = {
            "title", "domain", "tags", "task_id",
            "hypothesis", "hypothesis_status", "parent_experiment",
            "compared_to", "builds_on",
            "confidence", "evidence_count", "evidence_ids",
            "insight_type", "source_count",
            "status", "evidence_for", "evidence_against",
            "id", "created_at",
        }
        metadata = {k: v for k, v in params.items() if k in _VALID_METADATA_FIELDS}

        try:
            store = self._get_doc_store()
            doc_id = store.create(doc_type, metadata, body=body)

            # Auto-index embedding (best-effort, never blocks)
            try:
                EmbedderCls = _import_knowledge_embedder()
                with EmbedderCls(base_path=self._knowledge_base_path()) as embedder:
                    doc = store.get(doc_id)
                    if doc:
                        embedder.index_document(doc_id, doc.model_dump())
            except Exception as idx_err:
                logger.warning("Auto-index embedding failed for %s: %s", doc_id, idx_err)

            result = {
                "success": True,
                "doc_id": doc_id,
                "message": f"Document created: {doc_id}",
            }
            ec = EventCollector()
            ec.emit(KNOWLEDGE_RECORDED, SRC_KNOWLEDGE, {
                "doc_id": doc_id,
                "doc_type": doc_type,
                "title": title,
            })
            return ec.attach(result)
        except Exception as exc:
            return {"error": f"KnowledgeRecord failed: {exc}"}

    async def _handle_knowledge_get(self, params: dict[str, Any]) -> dict[str, Any]:
        """KnowledgeGet -> DocumentStore.get()."""
        doc_id = params.get("doc_id")
        if not doc_id:
            return {"error": "doc_id is required"}

        try:
            store = self._get_doc_store()
            doc = store.get(doc_id)
            if doc is None:
                return {"error": f"Document not found: {doc_id}"}

            backlinks = store.get_backlinks(doc_id)
            result = doc.model_dump()
            result["backlinks"] = backlinks
            return result
        except Exception as exc:
            return {"error": f"KnowledgeGet failed: {exc}"}

    # ======================================================================
    # GPU Methods
    # ======================================================================

    # DEPRECATED: Go native implementation preferred (fallback only)
    def _register_gpu_methods(self) -> None:
        self.methods["GPUStatus"] = self._handle_gpu_status
        self.methods["JobSubmit"] = self._handle_job_submit

    async def _handle_gpu_status(self, params: dict[str, Any]) -> dict[str, Any]:
        """GPUStatus -> get_gpu_monitor().get_all_gpus()."""
        try:
            monitor = GpuMonitor()  # GpuMonitor is actually get_gpu_monitor factory
            gpus = monitor.get_all_gpus()

            gpu_list = []
            for gpu in gpus:
                gpu_list.append({
                    "index": gpu.index,
                    "name": gpu.name,
                    "backend": gpu.backend.value if hasattr(gpu.backend, "value") else str(gpu.backend),
                    "total_vram_gb": round(gpu.vram_total_gb, 2),
                    "free_vram_gb": round(gpu.vram_free_gb, 2),
                    "utilization_pct": round(gpu.gpu_utilization, 1),
                })

            return {
                "gpu_count": len(gpu_list),
                "gpus": gpu_list,
                "backend": gpu_list[0]["backend"] if gpu_list else "cpu",
            }
        except Exception as exc:
            return {"error": f"GPUStatus failed: {exc}"}

    async def _handle_job_submit(self, params: dict[str, Any]) -> dict[str, Any]:
        """JobSubmit -> GpuJobScheduler.submit()."""
        command = params.get("command")
        if not command:
            return {"error": "command is required"}

        task_id = params.get("task_id")
        gpu_count = params.get("gpu_count", 1)
        working_dir = params.get("working_dir")

        try:
            scheduler = GpuJobScheduler()
            job = scheduler.submit(
                task_id=task_id or "manual",
                command=command,
                gpu_count=gpu_count,
                workdir=working_dir or ".",
            )
            return {
                "success": True,
                "job_id": job.job_id,
                "task_id": task_id,
                "message": f"Job submitted: {job.job_id}",
            }
        except Exception as exc:
            return {"error": f"JobSubmit failed: {exc}"}

    # ======================================================================
    # Onboard Methods
    # ======================================================================

    def _register_onboard_methods(self) -> None:
        self.methods["ProjectOnboard"] = self._handle_project_onboard

    async def _handle_project_onboard(self, params: dict[str, Any]) -> dict[str, Any]:
        """ProjectOnboard -> ProjectOnboarder.scan() + Knowledge store."""
        from c4.docs.onboarder import ProjectOnboarder

        max_files = params.get("max_files", 500)
        force = params.get("force", False)

        try:
            onboarder = ProjectOnboarder(
                project_root=self.project_root,
                max_files=max_files,
            )
            analysis = onboarder.scan()
            body = onboarder.render_markdown(analysis)

            # Save to knowledge store (update or create)
            doc_id = "pat-project-map"
            store = self._get_doc_store()
            existing = (store.docs_dir / f"{doc_id}.md").exists()

            if existing and not force:
                store.update(doc_id, body=body)
            else:
                # Remove existing to recreate with fixed ID
                existing_file = store.docs_dir / f"{doc_id}.md"
                if existing_file.exists():
                    existing_file.unlink()
                    # Clean from index too
                    try:
                        with store._get_conn() as conn:
                            conn.execute("DELETE FROM documents WHERE id = ?", (doc_id,))
                            conn.execute("DELETE FROM documents_fts WHERE id = ?", (doc_id,))
                    except Exception:
                        pass

                store.create("pattern", {
                    "id": doc_id,
                    "title": "Project Structure Map",
                    "tags": ["onboarding", "project-map", "auto-generated"],
                }, body=body)

            return {
                "success": True,
                "doc_id": doc_id,
                "languages": analysis["stats"]["total_files_scanned"],
                "symbols": analysis["stats"]["total_symbols"],
                "elapsed": analysis["stats"]["elapsed_seconds"],
                "frameworks": analysis.get("frameworks", []),
            }
        except Exception as exc:
            return {"error": f"ProjectOnboard failed: {exc}"}

    # ======================================================================
    # Research Methods
    # ======================================================================

    # DEPRECATED: Go native implementation preferred (fallback only)
    def _register_research_methods(self) -> None:
        self.methods["ResearchStart"] = self._handle_research_start
        self.methods["ResearchStatus"] = self._handle_research_status
        self.methods["ResearchRecord"] = self._handle_research_record
        self.methods["ResearchApprove"] = self._handle_research_approve
        self.methods["ResearchNext"] = self._handle_research_next

    def _research_base_path(self) -> Path:
        return self.project_root / ".c4" / "research"

    def _get_research_store(self) -> Any:
        """Get or create a cached ResearchStore instance."""
        if self._cached_research_store is None:
            self._cached_research_store = ResearchStore(base_path=self._research_base_path())
        return self._cached_research_store

    async def _handle_research_start(self, params: dict[str, Any]) -> dict[str, Any]:
        """ResearchStart -> create project + first iteration."""
        name = params.get("name")
        if not name:
            return {"error": "name is required"}

        try:
            store = self._get_research_store()
            project_id = store.create_project(
                name=name,
                paper_path=params.get("paper_path"),
                repo_path=params.get("repo_path"),
                target_score=params.get("target_score", 7.0),
            )
            iteration_id = store.create_iteration(project_id)
            result = {
                "success": True,
                "project_id": project_id,
                "iteration_id": iteration_id,
            }
            ec = EventCollector()
            ec.emit(RESEARCH_STARTED, SRC_RESEARCH, {
                "project_id": project_id,
                "name": name,
            })
            return ec.attach(result)
        except Exception as exc:
            return {"error": f"ResearchStart failed: {exc}"}

    async def _handle_research_status(self, params: dict[str, Any]) -> dict[str, Any]:
        """ResearchStatus -> project + iterations + current."""
        project_id = params.get("project_id")
        if not project_id:
            return {"error": "project_id is required"}

        try:
            store = self._get_research_store()
            project = store.get_project(project_id)
            if project is None:
                return {"error": f"Project not found: {project_id}"}

            iterations = store.list_iterations(project_id)
            current = store.get_current_iteration(project_id)

            return {
                "project": project.model_dump(mode="json"),
                "iterations": [i.model_dump(mode="json") for i in iterations],
                "current_iteration": current.model_dump(mode="json") if current else None,
            }
        except Exception as exc:
            return {"error": f"ResearchStatus failed: {exc}"}

    async def _handle_research_record(self, params: dict[str, Any]) -> dict[str, Any]:
        """ResearchRecord -> update current iteration with review/experiment data."""
        project_id = params.get("project_id")
        if not project_id:
            return {"error": "project_id is required"}

        try:
            store = self._get_research_store()
            current = store.get_current_iteration(project_id)
            if current is None:
                return {"error": "No active iteration"}

            update_kwargs: dict[str, Any] = {}
            for key in ("review_score", "axis_scores", "gaps", "experiments", "status"):
                if key in params:
                    update_kwargs[key] = params[key]

            if update_kwargs:
                store.update_iteration(current.id, **update_kwargs)

            result = {"success": True, "iteration_id": current.id}
            ec = EventCollector()
            ec.emit(RESEARCH_RECORDED, SRC_RESEARCH, {
                "project_id": project_id,
            })
            return ec.attach(result)
        except Exception as exc:
            return {"error": f"ResearchRecord failed: {exc}"}

    async def _handle_research_approve(self, params: dict[str, Any]) -> dict[str, Any]:
        """ResearchApprove -> continue/pause/complete project."""
        project_id = params.get("project_id")
        if not project_id:
            return {"error": "project_id is required"}
        action = params.get("action")
        if action not in ("continue", "pause", "complete"):
            return {"error": "action must be 'continue', 'pause', or 'complete'"}

        try:
            store = self._get_research_store()

            if action == "continue":
                store.update_project(project_id, status="active")
                # Mark current iteration as done and create new one
                current = store.get_current_iteration(project_id)
                if current and current.status != "done":
                    store.update_iteration(current.id, status="done")
                iteration_id = store.create_iteration(project_id)
                return {"success": True, "iteration_id": iteration_id}
            elif action == "pause":
                store.update_project(project_id, status="paused")
                return {"success": True}
            else:  # complete
                store.update_project(project_id, status="completed")
                current = store.get_current_iteration(project_id)
                if current and current.status != "done":
                    store.update_iteration(current.id, status="done")
                return {"success": True}
        except Exception as exc:
            return {"error": f"ResearchApprove failed: {exc}"}

    async def _handle_research_next(self, params: dict[str, Any]) -> dict[str, Any]:
        """ResearchNext -> suggest next action."""
        project_id = params.get("project_id")
        if not project_id:
            return {"error": "project_id is required"}

        try:
            store = self._get_research_store()
            return store.suggest_next(project_id)
        except Exception as exc:
            return {"error": f"ResearchNext failed: {exc}"}

    # ======================================================================
    # C2 Document Lifecycle Methods
    # ======================================================================

    # DEPRECATED: Go native implementation preferred (fallback only)
    def _register_c2_methods(self) -> None:
        self.methods["C2ParseDocument"] = self._handle_c2_parse_document
        self.methods["C2ExtractText"] = self._handle_c2_extract_text
        self.methods["C2WorkspaceCreate"] = self._handle_c2_workspace_create
        self.methods["C2WorkspaceLoad"] = self._handle_c2_workspace_load
        self.methods["C2WorkspaceSave"] = self._handle_c2_workspace_save
        self.methods["C2PersonaLearn"] = self._handle_c2_persona_learn
        self.methods["C2ProfileLoad"] = self._handle_c2_profile_load
        self.methods["C2ProfileSave"] = self._handle_c2_profile_save

    async def _handle_c2_parse_document(self, params: dict[str, Any]) -> dict[str, Any]:
        """C2ParseDocument -> c4.c2.converter.parse_document()."""
        file_path = params.get("file_path")
        if not file_path:
            return {"error": "file_path is required"}

        try:
            from c4.c2.converter import parse_document

            doc = parse_document(Path(file_path))
            result = {
                "blocks": [b.model_dump(mode="json") for b in doc.blocks],
                "metadata": doc.metadata.model_dump(mode="json") if doc.metadata else {},
                "block_count": len(doc.blocks),
            }
            ec = EventCollector()
            fmt_name = Path(file_path).suffix.lstrip(".") or "unknown"
            ec.emit(C2_DOCUMENT_PARSED, SRC_C2, {
                "file_path": file_path,
                "block_count": len(doc.blocks),
                "format": fmt_name,
            })
            return ec.attach(result)
        except Exception as exc:
            return {"error": f"C2ParseDocument failed: {exc}"}

    async def _handle_c2_extract_text(self, params: dict[str, Any]) -> dict[str, Any]:
        """C2ExtractText -> c4.c2.converter.extract_text()."""
        file_path = params.get("file_path")
        if not file_path:
            return {"error": "file_path is required"}

        try:
            from c4.c2.converter import extract_text

            text = extract_text(Path(file_path))
            result = {"text": text, "char_count": len(text)}
            ec = EventCollector()
            ec.emit(C2_TEXT_EXTRACTED, SRC_C2, {
                "file_path": file_path,
                "char_count": len(text),
            })
            return ec.attach(result)
        except Exception as exc:
            return {"error": f"C2ExtractText failed: {exc}"}

    async def _handle_c2_workspace_create(self, params: dict[str, Any]) -> dict[str, Any]:
        """C2WorkspaceCreate -> c4.c2.workspace.create_workspace()."""
        name = params.get("name")
        if not name:
            return {"error": "name is required"}

        try:
            from c4.c2.models import ProjectType
            from c4.c2.workspace import create_workspace

            project_type_str = params.get("project_type", "academic_paper")
            try:
                project_type = ProjectType(project_type_str)
            except ValueError:
                project_type = ProjectType.ACADEMIC_PAPER

            goal = params.get("goal", "")
            sections = params.get("sections")

            state = create_workspace(name, project_type, goal, sections=sections)
            return {"state": state.model_dump(mode="json")}
        except Exception as exc:
            return {"error": f"C2WorkspaceCreate failed: {exc}"}

    async def _handle_c2_workspace_load(self, params: dict[str, Any]) -> dict[str, Any]:
        """C2WorkspaceLoad -> c4.c2.workspace.parse_workspace()."""
        project_dir = params.get("project_dir")
        if not project_dir:
            return {"error": "project_dir is required"}

        try:
            from c4.c2.workspace import parse_workspace

            ws_path = Path(project_dir) / "c2_workspace.md"
            if not ws_path.exists():
                return {"error": f"Workspace not found: {ws_path}"}

            md_text = ws_path.read_text(encoding="utf-8")
            state = parse_workspace(md_text)
            return {"state": state.model_dump(mode="json")}
        except Exception as exc:
            return {"error": f"C2WorkspaceLoad failed: {exc}"}

    async def _handle_c2_workspace_save(self, params: dict[str, Any]) -> dict[str, Any]:
        """C2WorkspaceSave -> c4.c2.workspace.save_workspace()."""
        project_dir = params.get("project_dir")
        if not project_dir:
            return {"error": "project_dir is required"}
        state_data = params.get("state")
        if not state_data:
            return {"error": "state is required"}

        try:
            from c4.c2.models import WorkspaceState
            from c4.c2.workspace import save_workspace

            state = WorkspaceState.model_validate(state_data)
            saved_path = save_workspace(state, Path(project_dir))
            return {"success": True, "path": str(saved_path)}
        except Exception as exc:
            return {"error": f"C2WorkspaceSave failed: {exc}"}

    async def _handle_c2_persona_learn(self, params: dict[str, Any]) -> dict[str, Any]:
        """C2PersonaLearn -> c4.c2.persona.run_review_learning()."""
        draft_path = params.get("draft_path")
        final_path = params.get("final_path")
        if not draft_path or not final_path:
            return {"error": "draft_path and final_path are required"}

        try:
            from c4.c2.persona import run_review_learning

            profile_path = params.get("profile_path")
            auto_apply = params.get("auto_apply", False)

            diff = run_review_learning(
                Path(draft_path),
                Path(final_path),
                profile_path=Path(profile_path) if profile_path else None,
                auto_apply=auto_apply,
            )
            return {
                "summary": diff.summary,
                "new_patterns": [p.model_dump(mode="json") for p in diff.new_patterns],
                "tone_updates": diff.tone_updates,
                "structure_updates": diff.structure_updates,
            }
        except Exception as exc:
            return {"error": f"C2PersonaLearn failed: {exc}"}

    async def _handle_c2_profile_load(self, params: dict[str, Any]) -> dict[str, Any]:
        """C2ProfileLoad -> c4.c2.profile.load_profile()."""
        try:
            from c4.c2.profile import load_profile

            profile_path = params.get("profile_path")
            profile = load_profile(Path(profile_path) if profile_path else None)
            return {"profile": profile}
        except Exception as exc:
            return {"error": f"C2ProfileLoad failed: {exc}"}

    async def _handle_c2_profile_save(self, params: dict[str, Any]) -> dict[str, Any]:
        """C2ProfileSave -> c4.c2.profile.save_profile()."""
        data = params.get("data")
        if not data or not isinstance(data, dict):
            return {"error": "data (dict) is required"}

        try:
            from c4.c2.profile import save_profile

            profile_path = params.get("profile_path")
            save_profile(data, Path(profile_path) if profile_path else None)
            return {"success": True}
        except Exception as exc:
            return {"error": f"C2ProfileSave failed: {exc}"}
