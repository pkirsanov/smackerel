"""Contract tests for the disposable external compiler provider fixture."""

from __future__ import annotations

import importlib.util
from pathlib import Path


def load_fixture_module():
    path = Path(__file__).parents[2] / "tests" / "e2e" / "intent-compiler-provider" / "provider.py"
    spec = importlib.util.spec_from_file_location("intent_compiler_provider_fixture", path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_annotation_fixture_preserves_dotted_artifact_id_and_compiled_slots() -> None:
    fixture = load_fixture_module()
    artifact_id = "test-bug069005-artifact-20260720T235657.409"
    intent = fixture.compiled_intent(
        f"prepared artifact {artifact_id} yesterday; score four; flavor needed another layer of garlic"
    )

    assert intent["action_class"] == "state_mutation"
    assert intent["side_effect_class"] == "write"
    assert intent["slots"] == {
        "artifact_id": artifact_id,
        "interaction_type": "made_it",
        "rating": 4,
        "note": "needs more garlic",
    }


def test_barcelona_weather_fixture_carries_canonical_location_slot() -> None:
    fixture = load_fixture_module()
    intent = fixture.compiled_intent("what is the weather in Barcelona")

    assert intent["action_class"] == "external_lookup"
    assert intent["scenario_hint"] == "weather_query"
    assert intent["tool_hints"] == ["location_normalize", "weather_lookup"]
    assert intent["slots"] == {"location": {"raw": "Barcelona"}}
