"""C4 Bridge Server -- JSON-RPC over TCP for Go<->Python communication.

Listens on a TCP port and handles JSON-RPC requests from the Go MCP server.
This avoids the need for protoc-generated stubs while maintaining the same
communication pattern defined in c4-core/proto/c4_bridge.proto.

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

from c4.daemon.code_ops import CodeOps

logger = logging.getLogger(__name__)

# Type alias for method handlers
MethodHandler = Callable[[dict[str, Any]], Awaitable[dict[str, Any]]]

# ---------------------------------------------------------------------------
# Lazy imports -- resolved at first BridgeServer construction, not at module
# load time, to avoid hard failures when optional deps (pynvml, torch) are
# missing. The module-level names below serve as mock targets in tests:
#   patch("c4.bridge.grpc_server.DocumentStore")
#   patch("c4.bridge.grpc_server.KnowledgeSearcher")
#   patch("c4.bridge.grpc_server.GpuMonitor")
#   patch("c4.bridge.grpc_server.GpuJobScheduler")
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


def _ensure_imports() -> None:
    """Resolve lazy imports once. Safe to call multiple times."""
    global DocumentStore, KnowledgeSearcher, KnowledgeEmbedder, GpuMonitor, GpuJobScheduler
    if DocumentStore is None:
        try:
            DocumentStore = _import_document_store()
        except ImportError:
            pass
    if KnowledgeSearcher is None:
        try:
            KnowledgeSearcher = _import_knowledge_searcher()
        except ImportError:
            pass
    if KnowledgeEmbedder is None:
        try:
            KnowledgeEmbedder = _import_knowledge_embedder()
        except ImportError:
            pass
    if GpuMonitor is None:
        try:
            GpuMonitor = _import_gpu_monitor()
        except ImportError:
            pass
    if GpuJobScheduler is None:
        try:
            GpuJobScheduler = _import_gpu_scheduler()
        except ImportError:
            pass


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
            env_port = os.environ.get("C4_GRPC_PORT")
            self.port = int(env_port) if env_port else 50051

        self.project_root = project_root or Path.cwd()

        # Internal state
        self._server: asyncio.Server | None = None
        self._code_ops = self._create_code_ops()

        # Resolve lazy imports
        _ensure_imports()

        # Register all methods
        self.methods: dict[str, MethodHandler] = {}
        self._register_lsp_methods()
        self._register_knowledge_methods()
        self._register_gpu_methods()

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

                response = await self._process_line(line)
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

    async def _handle_find_symbol(self, params: dict[str, Any]) -> dict[str, Any]:
        """FindSymbol -> CodeOps.find_symbol()."""
        try:
            return self._code_ops.find_symbol(
                name_path_pattern=params.get("name", ""),
                relative_path=params.get("file_path", ""),
                include_body=params.get("include_body", False),
                depth=params.get("depth", 0),
            )
        except Exception as exc:
            return {"error": f"FindSymbol failed: {exc}"}

    async def _handle_get_symbols_overview(self, params: dict[str, Any]) -> dict[str, Any]:
        """GetSymbolsOverview -> CodeOps.get_symbols_overview()."""
        try:
            return self._code_ops.get_symbols_overview(
                relative_path=params.get("file_path", ""),
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

    # ======================================================================
    # Knowledge Methods
    # ======================================================================

    def _register_knowledge_methods(self) -> None:
        self.methods["KnowledgeSearch"] = self._handle_knowledge_search
        self.methods["KnowledgeRecord"] = self._handle_knowledge_record
        self.methods["KnowledgeGet"] = self._handle_knowledge_get

    def _knowledge_base_path(self) -> Path:
        return self.project_root / ".c4" / "knowledge"

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
            return {"count": len(results), "results": results}
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
            store = DocumentStore(base_path=self._knowledge_base_path())
            doc_id = store.create(doc_type, metadata, body=body)

            # Auto-index embedding (best-effort, never blocks)
            try:
                EmbedderCls = _import_knowledge_embedder()
                embedder = EmbedderCls(base_path=self._knowledge_base_path())
                doc = store.get(doc_id)
                if doc:
                    embedder.index_document(doc_id, doc.model_dump())
                embedder.close()
            except Exception:
                pass

            return {
                "success": True,
                "doc_id": doc_id,
                "message": f"Document created: {doc_id}",
            }
        except Exception as exc:
            return {"error": f"KnowledgeRecord failed: {exc}"}

    async def _handle_knowledge_get(self, params: dict[str, Any]) -> dict[str, Any]:
        """KnowledgeGet -> DocumentStore.get()."""
        doc_id = params.get("doc_id")
        if not doc_id:
            return {"error": "doc_id is required"}

        try:
            store = DocumentStore(base_path=self._knowledge_base_path())
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
