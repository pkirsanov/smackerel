"""F2 (redteam LLM-enrichment cold-load) — Ollama keep_alive wiring tests.

Root cause: the domain/synthesis model unloads between sparse captures (Ollama's
default keep_alive = 5m), so each capture pays a 22-45s cold-load that blows the
30s domain-extraction budget → truncated / invalid JSON. The fix keeps the model
resident by passing ``keep_alive`` on the ml sidecar's Ollama completions.

These are UNIT tests. They prove the smackerel-side contract — that each Ollama
completion is composed with (a) the ``ollama_chat/`` (/api/chat) prefix and
(b) the SST-owned ``keep_alive`` window at the request TOP LEVEL — not the live
prod latency (which only the orchestrator's redeploy can confirm). ``keep_alive``
is honored by Ollama ONLY at the top level of the request body, and litellm
forwards it there for ``ollama_chat/`` but buries it under ``options`` for the
legacy ``ollama/`` (/api/generate) transform (verified vs litellm 1.59.8 +
1.84.0) — so each capture test asserts, ADVERSARIALLY, that the legacy
generate prefix is NOT used.
"""

import ast
import asyncio
import json
import shutil
import sys
import types
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
import yaml

# The [dev] unit lane deliberately does NOT install the heavy real litellm, so
# app.domain / app.processor (which `import litellm` at module top) can only be
# imported once a stand-in module exists. Mirror the guard the sibling LLM test
# modules use so this file is self-sufficient when run in isolation, not merely
# when a sibling injected the stub first (collection-order independence).
if "litellm" not in sys.modules:
    _mock_litellm = types.ModuleType("litellm")
    _mock_litellm.acompletion = None  # type: ignore[attr-defined]
    sys.modules["litellm"] = _mock_litellm

    _mock_exc = types.ModuleType("litellm.exceptions")
    _mock_exc.RateLimitError = type("RateLimitError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.ServiceUnavailableError = type("ServiceUnavailableError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.InternalServerError = type("InternalServerError", (Exception,), {})  # type: ignore[attr-defined]
    sys.modules["litellm.exceptions"] = _mock_exc

# --------------------------------------------------------------------------
# SST fail-loud resolver (ml/app/ollama_keepalive.py)
# --------------------------------------------------------------------------


def test_resolve_returns_configured_window(monkeypatch):
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "45m")
    from app.ollama_keepalive import resolve_ollama_keep_alive

    assert resolve_ollama_keep_alive() == "45m"


def test_resolve_fail_loud_when_unset(monkeypatch):
    # ADVERSARIAL — no default; a missing window must raise, never silently
    # substitute a fallback (smackerel NO-DEFAULTS / Gate G028). Fails if a
    # default is ever added to resolve_ollama_keep_alive().
    monkeypatch.delenv("ML_OLLAMA_KEEP_ALIVE", raising=False)
    from app.ollama_keepalive import resolve_ollama_keep_alive

    with pytest.raises(RuntimeError):
        resolve_ollama_keep_alive()


def test_resolve_fail_loud_when_blank(monkeypatch):
    # A whitespace-only value is as good as unset — must still fail loud.
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "   ")
    from app.ollama_keepalive import resolve_ollama_keep_alive

    with pytest.raises(RuntimeError):
        resolve_ollama_keep_alive()


@pytest.mark.parametrize("value", ["0", "-1", "0s", "-30m"])
def test_resolve_keep_alive_rejects_non_positive_spec102(monkeypatch, value):
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", value)
    from app.ollama_keepalive import resolve_ollama_keep_alive

    with pytest.raises(RuntimeError, match="positive"):
        resolve_ollama_keep_alive()


# --------------------------------------------------------------------------
# Spec 102 SCOPE-102-03 — SST per-model num_ctx + output-token budget resolvers
# --------------------------------------------------------------------------


def test_resolve_num_ctx_returns_sst_per_model_value(monkeypatch):
    monkeypatch.setenv(
        "ML_MODEL_MEMORY_PROFILES_JSON",
        '[{"model":"qwen3:30b-a3b","weights_mib":18432,"kv_mib_per_1k_ctx":102,"num_ctx":32768},'
        '{"model":"gemma4:26b","weights_mib":16384,"kv_mib_per_1k_ctx":256,"num_ctx":8192}]',
    )
    from app.ollama_keepalive import resolve_ollama_num_ctx

    assert resolve_ollama_num_ctx("qwen3:30b-a3b") == 32768
    assert resolve_ollama_num_ctx("gemma4:26b") == 8192


@pytest.mark.parametrize(
    "profiles_json",
    [
        None,
        "",
        "not-json",
        '{"model":"qwen3:30b-a3b","num_ctx":32768}',
        '[{"model":"qwen3:30b-a3b","num_ctx":0}]',
        '[{"model":"qwen3:30b-a3b","num_ctx":true}]',
        '[{"model":"qwen3:30b-a3b","num_ctx":32768},{"model":"ollama_chat/qwen3:30b-a3b","num_ctx":8192}]',
    ],
)
def test_resolve_num_ctx_fails_loud_for_invalid_profile_set(monkeypatch, profiles_json):
    if profiles_json is None:
        monkeypatch.delenv("ML_MODEL_MEMORY_PROFILES_JSON", raising=False)
    else:
        monkeypatch.setenv("ML_MODEL_MEMORY_PROFILES_JSON", profiles_json)
    from app.ollama_keepalive import resolve_ollama_num_ctx

    with pytest.raises(RuntimeError, match="ML_MODEL_MEMORY_PROFILES_JSON"):
        resolve_ollama_num_ctx("qwen3:30b-a3b")


def test_unprofiled_selected_ollama_model_fails_before_dispatch_spec102(monkeypatch):
    monkeypatch.setenv(
        "ML_MODEL_MEMORY_PROFILES_JSON",
        '[{"model":"qwen3:30b-a3b","weights_mib":18432,"kv_mib_per_1k_ctx":102,"num_ctx":32768}]',
    )
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    from app.ollama_keepalive import apply_ollama_profile_to_litellm

    network_call = MagicMock()
    with pytest.raises(RuntimeError, match="missing_model_profile"):
        kwargs = apply_ollama_profile_to_litellm(
            {"model": "ollama_chat/not-profiled", "messages": []},
            provider="ollama",
            model="not-profiled",
        )
        network_call(**kwargs)

    network_call.assert_not_called()


def test_profile_errors_redact_supplied_values_security102(monkeypatch):
    sentinel = "SENTINEL-PROFILE-SECRET-RR03"
    from app.ollama_keepalive import resolve_ollama_keep_alive, resolve_ollama_num_ctx

    monkeypatch.setenv(
        "ML_MODEL_MEMORY_PROFILES_JSON",
        f"[{json.dumps({'model': 'qwen3:30b-a3b', 'num_ctx': sentinel})}]",
    )
    with pytest.raises(RuntimeError) as num_ctx_error:
        resolve_ollama_num_ctx("qwen3:30b-a3b")
    assert sentinel not in str(num_ctx_error.value)
    assert "category=" in str(num_ctx_error.value)
    assert "key=num_ctx" in str(num_ctx_error.value)

    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", sentinel)
    with pytest.raises(RuntimeError) as keep_alive_error:
        resolve_ollama_keep_alive()
    assert sentinel not in str(keep_alive_error.value)
    assert "category=" in str(keep_alive_error.value)
    assert "key=ML_OLLAMA_KEEP_ALIVE" in str(keep_alive_error.value)

    monkeypatch.setenv(
        "ML_MODEL_MEMORY_PROFILES_JSON",
        '[{"model":"qwen3:30b-a3b","num_ctx":32768}]',
    )
    with pytest.raises(RuntimeError) as model_error:
        resolve_ollama_num_ctx(sentinel)
    assert sentinel not in str(model_error.value)
    assert "category=" in str(model_error.value)
    assert "model=<redacted>" in str(model_error.value)


def test_litellm_dispatch_requires_resolved_profile_before_network_spec102():
    from app.ollama_keepalive import dispatch_litellm

    calls = {"count": 0}

    async def capture(**kwargs):
        calls["count"] += 1
        return object()

    with pytest.raises(RuntimeError, match="resolved OllamaRequestProfile"):
        asyncio.run(
            dispatch_litellm(
                {"model": "ollama_chat/qwen3:30b-a3b", "messages": []},
                provider="ollama",
                model="qwen3:30b-a3b",
                profile=None,
                completion_fn=capture,
            )
        )
    assert calls["count"] == 0


def test_litellm_dispatch_rejects_payload_profile_model_mismatch_before_network_spec102(monkeypatch):
    from app.ollama_keepalive import dispatch_litellm, resolve_ollama_request_profile

    calls = {"count": 0}

    async def capture(**kwargs):
        calls["count"] += 1
        return object()

    profile = resolve_ollama_request_profile("qwen3:30b-a3b")
    with pytest.raises(RuntimeError, match="profile_model_mismatch") as exc_info:
        asyncio.run(
            dispatch_litellm(
                {"model": "ollama_chat/gemma4:26b", "messages": []},
                provider="ollama",
                model="qwen3:30b-a3b",
                profile=profile,
                completion_fn=capture,
            )
        )

    assert "gemma4:26b" not in str(exc_info.value)
    assert "qwen3:30b-a3b" not in str(exc_info.value)
    assert calls["count"] == 0


def test_litellm_adapter_merges_options_and_keep_alive_spec102(monkeypatch):
    monkeypatch.setenv(
        "ML_MODEL_MEMORY_PROFILES_JSON",
        '[{"model":"qwen3:30b-a3b","weights_mib":18432,"kv_mib_per_1k_ctx":102,"num_ctx":32768}]',
    )
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "45m")
    from app.ollama_keepalive import apply_ollama_profile_to_litellm

    kwargs = {
        "model": "ollama_chat/qwen3:30b-a3b",
        "messages": [{"role": "user", "content": "hello"}],
        "think": False,
        "tools": [{"type": "function"}],
        "response_format": {"type": "json_object"},
        "max_tokens": 1234,
        "temperature": 0.1,
        "options": {"seed": 7, "top_k": 20},
    }

    result = apply_ollama_profile_to_litellm(kwargs, provider="ollama", model="qwen3:30b-a3b")

    assert result["options"] == {"seed": 7, "top_k": 20, "num_ctx": 32768}
    assert result["keep_alive"] == "45m"
    assert result["think"] is False
    assert result["tools"] == [{"type": "function"}]
    assert result["response_format"] == {"type": "json_object"}
    assert result["max_tokens"] == 1234
    assert result["temperature"] == 0.1


def test_native_json_adapter_merges_options_and_keep_alive_spec102(monkeypatch):
    monkeypatch.setenv(
        "ML_MODEL_MEMORY_PROFILES_JSON",
        '[{"model":"gemma4:26b","weights_mib":16384,"kv_mib_per_1k_ctx":256,"num_ctx":8192}]',
    )
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    from app.ollama_keepalive import apply_ollama_profile_to_native_json

    payload = {
        "model": "gemma4:26b",
        "prompt": "describe this image",
        "stream": False,
        "think": False,
        "options": {"temperature": 0, "seed": 11},
    }

    result = apply_ollama_profile_to_native_json(payload, provider="ollama", model="gemma4:26b")

    assert result["options"] == {"temperature": 0, "seed": 11, "num_ctx": 8192}
    assert result["keep_alive"] == "30m"
    assert result["think"] is False
    assert result["prompt"] == "describe this image"
    assert result["stream"] is False


def test_hosted_provider_adapter_excludes_ollama_fields_spec102(monkeypatch):
    monkeypatch.delenv("ML_MODEL_MEMORY_PROFILES_JSON", raising=False)
    monkeypatch.delenv("ML_OLLAMA_KEEP_ALIVE", raising=False)
    from app.ollama_keepalive import apply_ollama_profile_to_litellm

    kwargs = {"model": "gpt-4o", "messages": [], "options": {"seed": 5}}
    result = apply_ollama_profile_to_litellm(kwargs, provider="openai", model="gpt-4o")

    assert result == kwargs
    assert "keep_alive" not in result
    assert result["options"] == {"seed": 5}


_EXPECTED_PROFILED_BUILDERS = {
    ("agent.py", "handle_invoke"): "dispatch_litellm",
    ("card_categories.py", "extract_card_categories"): "dispatch_litellm",
    ("domain.py", "_do_domain_extract"): "dispatch_litellm",
    ("drive_classify.py", "classify_drive_file"): "dispatch_litellm",
    ("intelligence.py", "_call_llm"): "dispatch_ollama_native_json_async",
    ("main.py", "_warmup_domain_model"): "dispatch_litellm",
    ("nats_client.py", "_handle_search_rerank"): "dispatch_litellm",
    ("nats_client.py", "_handle_digest_generate"): "dispatch_litellm",
    ("ocr.py", "extract_text_ollama"): "dispatch_ollama_native_json",
    ("processor.py", "process_content"): "dispatch_litellm",
    ("routes/chat.py", "_dispatch_ollama"): "dispatch_litellm",
    ("synthesis.py", "handle_extract"): "dispatch_litellm",
    ("synthesis.py", "handle_crosssource"): "dispatch_litellm",
}


def _assert_profiled_dispatch_structure(app_root: Path) -> None:
    """Reject direct Ollama network primitives outside the foundation.

    Symbol provenance is tracked through module and function lexical scopes,
    imports, assignments, client construction, bound methods, and endpoint
    aliases. The guard therefore reasons about ownership of the network
    primitive rather than source spelling or statement adjacency.
    """
    dispatch_calls: dict[tuple[str, str], list[tuple[str, int]]] = {}
    direct_network_refs: list[tuple[str, str, str, int]] = []

    dispatchers = {
        "dispatch_litellm",
        "dispatch_ollama_native_json",
        "dispatch_ollama_native_json_async",
    }

    def dispatcher_symbol(name: str) -> str:
        return f"dispatcher:{name}"

    class BuilderVisitor(ast.NodeVisitor):
        def __init__(self, relative_path: str):
            self.relative_path = relative_path
            self.functions: list[str] = []
            self.scopes: list[dict[str, str]] = [{}]

        def _bind(self, name: str, symbol: str | None) -> None:
            if symbol is None:
                self.scopes[-1].pop(name, None)
            else:
                self.scopes[-1][name] = symbol

        def _lookup(self, name: str) -> str | None:
            for scope in reversed(self.scopes):
                if name in scope:
                    return scope[name]
            return None

        def _resolve(self, node: ast.AST) -> str | None:
            if isinstance(node, ast.Name):
                return self._lookup(node.id)
            if not isinstance(node, ast.Attribute):
                return None

            owner = self._resolve(node.value)
            if owner == "module:litellm" and node.attr == "acompletion":
                return "network:litellm.acompletion"
            if owner in {"module:requests", "module:httpx"} and node.attr == "post":
                return "network:http.post"
            if owner in {"module:requests", "module:httpx"} and node.attr in {
                "Session",
                "Client",
                "AsyncClient",
            }:
                return "factory:http.client"
            if owner == "instance:http.client" and node.attr == "post":
                return "network:http.post"
            return None

        def _assignment_symbol(self, value: ast.AST) -> str | None:
            resolved = self._resolve(value)
            if resolved is not None:
                return resolved
            if isinstance(value, ast.Call) and self._resolve(value.func) == "factory:http.client":
                return "instance:http.client"
            if any(
                isinstance(child, ast.Constant) and isinstance(child.value, str) and "/api/generate" in child.value
                for child in ast.walk(value)
            ):
                return "endpoint:ollama.generate"
            return None

        def _bind_target(self, target: ast.AST, symbol: str | None) -> None:
            if isinstance(target, ast.Name):
                self._bind(target.id, symbol)
            elif isinstance(target, (ast.Tuple, ast.List)):
                for element in target.elts:
                    self._bind_target(element, None)

        def _visit_function(self, node: ast.FunctionDef | ast.AsyncFunctionDef) -> None:
            self.functions.append(node.name)
            function_scope: dict[str, str] = {}
            all_args = [
                *node.args.posonlyargs,
                *node.args.args,
                *node.args.kwonlyargs,
            ]
            if node.args.vararg is not None:
                all_args.append(node.args.vararg)
            if node.args.kwarg is not None:
                all_args.append(node.args.kwarg)
            for argument in all_args:
                if argument.arg == "completion_fn":
                    function_scope[argument.arg] = "network:completion_fn"
            self.scopes.append(function_scope)
            for statement in node.body:
                self.visit(statement)
            self.scopes.pop()
            self.functions.pop()

        def visit_FunctionDef(self, node: ast.FunctionDef) -> None:
            self._visit_function(node)

        def visit_AsyncFunctionDef(self, node: ast.AsyncFunctionDef) -> None:
            self._visit_function(node)

        def visit_Import(self, node: ast.Import) -> None:
            for alias in node.names:
                if alias.name in {"litellm", "requests", "httpx"}:
                    self._bind(alias.asname or alias.name, f"module:{alias.name}")

        def visit_ImportFrom(self, node: ast.ImportFrom) -> None:
            module = node.module or ""
            for alias in node.names:
                local_name = alias.asname or alias.name
                if module == "litellm" and alias.name == "acompletion":
                    self._bind(local_name, "network:litellm.acompletion")
                elif module in {"requests", "httpx"} and alias.name == "post":
                    self._bind(local_name, "network:http.post")
                elif module in {"requests", "httpx"} and alias.name in {
                    "Session",
                    "Client",
                    "AsyncClient",
                }:
                    self._bind(local_name, "factory:http.client")
                elif module.endswith("ollama_keepalive") and alias.name in dispatchers:
                    self._bind(local_name, dispatcher_symbol(alias.name))

        def visit_Assign(self, node: ast.Assign) -> None:
            symbol = self._assignment_symbol(node.value)
            for target in node.targets:
                self._bind_target(target, symbol)
            self.visit(node.value)

        def visit_AnnAssign(self, node: ast.AnnAssign) -> None:
            symbol = self._assignment_symbol(node.value) if node.value is not None else None
            self._bind_target(node.target, symbol)
            if node.value is not None:
                self.visit(node.value)

        def _visit_with(self, node: ast.With | ast.AsyncWith) -> None:
            for item in node.items:
                symbol = self._assignment_symbol(item.context_expr)
                self._bind_target(item.optional_vars, symbol)
                self.visit(item.context_expr)
            for statement in node.body:
                self.visit(statement)

        def visit_With(self, node: ast.With) -> None:
            self._visit_with(node)

        def visit_AsyncWith(self, node: ast.AsyncWith) -> None:
            self._visit_with(node)

        def visit_Call(self, node: ast.Call) -> None:
            if not self.functions:
                self.generic_visit(node)
                return
            key = (self.relative_path, self.functions[-1])
            called_symbol = self._resolve(node.func)

            if called_symbol is not None and called_symbol.startswith("dispatcher:"):
                dispatcher = called_symbol.removeprefix("dispatcher:")
                profile_keyword = next((kw.value for kw in node.keywords if kw.arg == "profile"), None)
                explicitly_hosted = (
                    dispatcher == "dispatch_litellm"
                    and isinstance(profile_keyword, ast.Constant)
                    and profile_keyword.value is None
                )
                if not explicitly_hosted:
                    dispatch_calls.setdefault(key, []).append((dispatcher, node.lineno))

            if called_symbol in {"network:litellm.acompletion", "network:completion_fn"}:
                direct_network_refs.append((*key, called_symbol.removeprefix("network:"), node.lineno))
            if called_symbol == "network:http.post" and node.args:
                endpoint_symbol = self._resolve(node.args[0])
                has_generate_literal = any(
                    isinstance(child, ast.Constant) and isinstance(child.value, str) and "/api/generate" in child.value
                    for child in ast.walk(node.args[0])
                )
                if endpoint_symbol == "endpoint:ollama.generate" or has_generate_literal:
                    direct_network_refs.append((*key, "http.post:/api/generate", node.lineno))
            self.generic_visit(node)

    for source_path in sorted(app_root.rglob("*.py")):
        relative_path = source_path.relative_to(app_root).as_posix()
        if relative_path == "ollama_keepalive.py":
            continue
        BuilderVisitor(relative_path).visit(ast.parse(source_path.read_text(), filename=str(source_path)))

    assert not direct_network_refs, f"direct Ollama network primitive outside foundation: {direct_network_refs}"
    assert len(_EXPECTED_PROFILED_BUILDERS) == 13
    assert set(dispatch_calls) == set(_EXPECTED_PROFILED_BUILDERS), (
        "profiled builder inventory mismatch: "
        f"missing={sorted(set(_EXPECTED_PROFILED_BUILDERS) - set(dispatch_calls))} "
        f"extra={sorted(set(dispatch_calls) - set(_EXPECTED_PROFILED_BUILDERS))}"
    )
    for builder, dispatcher in _EXPECTED_PROFILED_BUILDERS.items():
        calls = dispatch_calls[builder]
        assert len(calls) == 1 and calls[0][0] == dispatcher, (
            f"builder {builder} must call exactly one {dispatcher}; got {dispatch_calls.get(builder)}"
        )


def test_production_ollama_builder_inventory_is_complete_spec102():
    """TP-C3-14 / SEC-102-RR-02: 13 builders own no network primitive."""
    _assert_profiled_dispatch_structure(Path(__file__).resolve().parents[1] / "app")


def test_structural_guard_rejects_aliased_litellm_dispatch_spec102(tmp_path):
    app_root = Path(__file__).resolve().parents[1] / "app"
    mutated_root = tmp_path / "app"
    shutil.copytree(app_root, mutated_root)
    target = mutated_root / "agent.py"
    source = target.read_text()
    anchor = "response = await dispatch_litellm("
    assert anchor in source
    target.write_text(
        source.replace(
            anchor,
            "send = litellm.acompletion\n        response = await send(",
            1,
        )
    )

    with pytest.raises(AssertionError, match="direct Ollama network primitive"):
        _assert_profiled_dispatch_structure(mutated_root)


def test_structural_guard_rejects_module_level_imported_alias_spec102(tmp_path):
    app_root = Path(__file__).resolve().parents[1] / "app"
    mutated_root = tmp_path / "app"
    shutil.copytree(app_root, mutated_root)
    target = mutated_root / "agent.py"
    source = target.read_text()
    import_anchor = "from __future__ import annotations\n"
    call_anchor = "response = await dispatch_litellm("
    assert import_anchor in source
    assert call_anchor in source
    target.write_text(
        source.replace(
            import_anchor,
            "from __future__ import annotations\n\nfrom litellm import acompletion as module_send\n",
            1,
        ).replace(call_anchor, "response = await module_send(", 1)
    )

    with pytest.raises(AssertionError, match="direct Ollama network primitive"):
        _assert_profiled_dispatch_structure(mutated_root)


def test_structural_guard_rejects_bound_native_post_alias_spec102(tmp_path):
    app_root = Path(__file__).resolve().parents[1] / "app"
    mutated_root = tmp_path / "app"
    shutil.copytree(app_root, mutated_root)
    target = mutated_root / "ocr.py"
    source = target.read_text()
    import_anchor = "import os\n"
    call_anchor = "response = dispatch_ollama_native_json("
    assert import_anchor in source
    assert call_anchor in source
    target.write_text(
        source.replace(import_anchor, "import os\nimport requests\n", 1).replace(
            call_anchor,
            "session = requests.Session()\n        send = session.post\n        response = send(",
            1,
        )
    )

    with pytest.raises(AssertionError, match="direct Ollama network primitive"):
        _assert_profiled_dispatch_structure(mutated_root)


def test_structural_guard_rejects_extra_dispatch_builder_spec102(tmp_path):
    app_root = Path(__file__).resolve().parents[1] / "app"
    mutated_root = tmp_path / "app"
    shutil.copytree(app_root, mutated_root)
    target = mutated_root / "agent.py"
    source = target.read_text()
    import_anchor = "from __future__ import annotations\n"
    assert import_anchor in source
    target.write_text(
        source.replace(
            import_anchor,
            "from __future__ import annotations\n\n"
            "from .ollama_keepalive import dispatch_litellm, resolve_ollama_request_profile\n",
            1,
        )
        + "\n\nasync def unregistered_profiled_builder():\n"
        + "    return await dispatch_litellm(\n"
        + "        {'model': 'ollama_chat/gemma4:26b', 'messages': []},\n"
        + "        provider='ollama', model='gemma4:26b',\n"
        + "        profile=resolve_ollama_request_profile('gemma4:26b'),\n"
        + "    )\n"
    )

    with pytest.raises(AssertionError, match="builder inventory mismatch"):
        _assert_profiled_dispatch_structure(mutated_root)


def test_structural_guard_rejects_nominal_dispatch_plus_direct_alias_spec102(tmp_path):
    app_root = Path(__file__).resolve().parents[1] / "app"
    mutated_root = tmp_path / "app"
    shutil.copytree(app_root, mutated_root)
    target = mutated_root / "agent.py"
    source = target.read_text()
    import_anchor = "from __future__ import annotations\n"
    call_anchor = "response = await dispatch_litellm("
    assert import_anchor in source
    assert call_anchor in source
    target.write_text(
        source.replace(
            import_anchor,
            "from __future__ import annotations\n\nfrom litellm import acompletion as module_send\n",
            1,
        ).replace(
            call_anchor,
            "await module_send(model='ollama_chat/gemma4:26b', messages=[])\n"
            "        response = await dispatch_litellm(",
            1,
        )
    )

    with pytest.raises(AssertionError, match="direct Ollama network primitive"):
        _assert_profiled_dispatch_structure(mutated_root)


def test_structural_guard_rejects_discarded_profile_result_spec102(tmp_path):
    app_root = Path(__file__).resolve().parents[1] / "app"
    mutated_root = tmp_path / "app"
    shutil.copytree(app_root, mutated_root)
    target = mutated_root / "agent.py"
    source = target.read_text()
    anchor = "response = await dispatch_litellm("
    assert anchor in source
    target.write_text(
        source.replace(
            anchor,
            "_profiled_but_discarded = apply_ollama_profile_to_litellm("
            "completion_kwargs, provider=provider, model=model)\n"
            "        response = await litellm.acompletion(",
            1,
        )
    )

    with pytest.raises(AssertionError, match="direct Ollama network primitive"):
        _assert_profiled_dispatch_structure(mutated_root)


def test_resolve_domain_output_budget_reads_sst(monkeypatch):
    monkeypatch.setenv("ML_DOMAIN_OUTPUT_TOKEN_BUDGET", "4096")
    from app.ollama_keepalive import resolve_domain_output_token_budget

    assert resolve_domain_output_token_budget() == 4096


def test_resolve_domain_output_budget_fail_loud_when_unset(monkeypatch):
    # ADVERSARIAL — no default; a missing budget must raise, never silently
    # substitute 2000 or any other fallback (BUG-026-006 / Gate G028).
    monkeypatch.delenv("ML_DOMAIN_OUTPUT_TOKEN_BUDGET", raising=False)
    from app.ollama_keepalive import resolve_domain_output_token_budget

    with pytest.raises(RuntimeError):
        resolve_domain_output_token_budget()


def test_resolve_domain_output_budget_fail_loud_on_nonint(monkeypatch):
    monkeypatch.setenv("ML_DOMAIN_OUTPUT_TOKEN_BUDGET", "lots")
    from app.ollama_keepalive import resolve_domain_output_token_budget

    with pytest.raises(RuntimeError):
        resolve_domain_output_token_budget()


# --------------------------------------------------------------------------
# domain.py — keep_alive rides the ollama_chat/ completion
# --------------------------------------------------------------------------


def test_domain_extract_passes_keepalive_via_ollama_chat(monkeypatch):
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "45m")
    from app.domain import handle_domain_extract

    captured: dict = {}

    def _capture(**kwargs):
        captured.update(kwargs)
        resp = MagicMock()
        resp.choices = [
            MagicMock(message=MagicMock(content=json.dumps({"domain": "recipe", "ingredients": [], "steps": []})))
        ]
        resp.usage = MagicMock(total_tokens=12)
        return resp

    with patch("app.domain.litellm.acompletion", new_callable=AsyncMock) as mock_comp:
        mock_comp.side_effect = _capture
        data = {
            "artifact_id": "art-1",
            "contract_version": "recipe-extraction-v1",
            "content_type": "recipe",
            "content_raw": "Ingredients: flour. Instructions: bake.",
        }
        result = asyncio.run(handle_domain_extract(data, "ollama", "gemma4:26b", "", "http://ollama:11434"))

    assert result["success"] is True
    assert captured["model"] == "ollama_chat/gemma4:26b"
    assert captured["keep_alive"] == "45m"
    assert captured["api_base"] == "http://ollama:11434"
    # ADVERSARIAL: the legacy ollama/ generate prefix (where litellm buries
    # keep_alive under `options`, making it a silent no-op) must NOT be used.
    assert not captured["model"].startswith("ollama/")


# --------------------------------------------------------------------------
# processor.py — keep_alive rides the ollama_chat/ completion
# --------------------------------------------------------------------------


def test_process_content_passes_keepalive_via_ollama_chat(monkeypatch):
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    from app.processor import process_content

    captured: dict = {}

    def _capture(**kwargs):
        captured.update(kwargs)
        resp = MagicMock()
        resp.choices = [MagicMock(message=MagicMock(content=json.dumps({"artifact_type": "article", "title": "T"})))]
        resp.usage = MagicMock(total_tokens=20)
        return resp

    with patch("app.processor.litellm") as mock_litellm:
        mock_litellm.acompletion = AsyncMock(side_effect=_capture)
        result = asyncio.run(
            process_content(
                content="hello world",
                content_type="article",
                source_id="s1",
                processing_tier="standard",
                user_context="",
                model="gemma4:26b",
                api_key="",
                provider="ollama",
            )
        )

    assert result["success"] is True
    assert captured["model"] == "ollama_chat/gemma4:26b"
    assert captured["keep_alive"] == "30m"
    assert captured["api_base"] == "http://ollama:11434"
    assert not captured["model"].startswith("ollama/")


# --------------------------------------------------------------------------
# synthesis.py — keep_alive rides the ollama_chat/ completion
# --------------------------------------------------------------------------


def test_synthesis_extract_passes_keepalive_via_ollama_chat(monkeypatch, tmp_path):
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    contract = {
        "version": "ingest-synthesis-v1",
        "type": "ingest-synthesis",
        "system_prompt": "You are a knowledge synthesis engine.",
        "extraction_schema": {"type": "object"},
        "validation_rules": {},
        "token_budget": 500,
        "temperature": 0.3,
    }
    (tmp_path / "ingest-synthesis-v1.yaml").write_text(yaml.dump(contract))
    monkeypatch.setenv("PROMPT_CONTRACTS_DIR", str(tmp_path))
    from app.synthesis import handle_extract

    captured: dict = {}

    def _capture(**kwargs):
        captured.update(kwargs)
        resp = MagicMock()
        resp.choices = [MagicMock(message=MagicMock(content="{}"))]
        resp.usage = MagicMock(total_tokens=5)
        resp.model = "ollama_chat/gemma4:26b"
        return resp

    mock_litellm = MagicMock()
    mock_litellm.acompletion = AsyncMock(side_effect=_capture)
    with patch.dict(sys.modules, {"litellm": mock_litellm}):
        result = asyncio.run(
            handle_extract(
                {"artifact_id": "a1", "prompt_contract_version": "ingest-synthesis-v1", "content_raw": "hello"},
                "ollama",
                "gemma4:26b",
                "",
                "http://ollama:11434",
            )
        )

    assert result["success"] is True
    assert captured["model"] == "ollama_chat/gemma4:26b"
    assert captured["keep_alive"] == "30m"
    assert captured["api_base"] == "http://ollama:11434"
    assert not captured["model"].startswith("ollama/")


# --------------------------------------------------------------------------
# main.py — startup warmup (best-effort, non-fatal)
# --------------------------------------------------------------------------


def test_warmup_skipped_for_non_ollama_provider():
    # No litellm call at all for hosted providers — returns immediately.
    from app.main import _warmup_domain_model

    asyncio.run(_warmup_domain_model({"LLM_PROVIDER": "openai", "LLM_MODEL": "gpt-4o", "OLLAMA_URL": ""}))


def test_warmup_uses_ollama_chat_and_keepalive(monkeypatch):
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    import litellm

    from app.main import _warmup_domain_model

    captured: dict = {}

    async def _capture(**kwargs):
        captured.update(kwargs)
        return MagicMock()

    monkeypatch.setattr(litellm, "acompletion", _capture, raising=False)
    asyncio.run(
        _warmup_domain_model({"LLM_PROVIDER": "ollama", "LLM_MODEL": "gemma4:26b", "OLLAMA_URL": "http://ollama:11434"})
    )

    assert captured["model"] == "ollama_chat/gemma4:26b"
    assert captured["keep_alive"] == "30m"
    assert captured["api_base"] == "http://ollama:11434"
    assert not captured["model"].startswith("ollama/")


def test_warmup_is_non_fatal_when_ollama_unreachable(monkeypatch):
    # ADVERSARIAL — a warmup failure at boot (model not pulled, Ollama down)
    # MUST be swallowed; it must NEVER propagate and block sidecar startup.
    # Fails if the try/except around the warmup completion is removed.
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    import litellm

    from app.main import _warmup_domain_model

    async def _boom(**kwargs):
        raise ConnectionError("connection refused")

    monkeypatch.setattr(litellm, "acompletion", _boom, raising=False)
    # Absence of a raised exception here IS the assertion.
    asyncio.run(
        _warmup_domain_model({"LLM_PROVIDER": "ollama", "LLM_MODEL": "gemma4:26b", "OLLAMA_URL": "http://ollama:11434"})
    )
