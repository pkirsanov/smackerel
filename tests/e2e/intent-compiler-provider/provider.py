"""Disposable external LLM-provider fixture for required assistant E2E."""

from __future__ import annotations

import json
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def compiled_intent(text: str) -> dict[str, object]:
    normalized = text.strip().lower()
    base: dict[str, object] = {
        "version": "v1",
        "language": "en",
        "user_goal": normalized,
        "scenario_hint": None,
        "tool_hints": [],
        "normalized_request": {"query": text},
        "slots": {},
        "missing_slots": [],
        "confidence": 0.99,
        "clarification_prompt": None,
        "safety_flags": [],
        "source_policy": {"requires_citations": True, "allowed_source_kinds": ["tool"]},
    }
    if "selected location:" in normalized:
        selected = json.loads(text.split("Selected location:", 1)[1].strip())
        base.update(
            action_class="external_lookup",
            side_effect_class="external_read",
            scenario_hint="weather_query",
            tool_hints=["weather_lookup"],
            slots={"location": selected},
        )
    elif "springfield" in normalized and "weather" in normalized:
        base.update(
            action_class="clarify",
            side_effect_class="none",
            scenario_hint="weather_query",
            tool_hints=["location_normalize", "weather_lookup"],
            slots={"location": {"raw": "Springfield"}},
            missing_slots=["location"],
            clarification_prompt="Which Springfield did you mean?",
        )
    elif "shopping list" in normalized:
        item_match = re.search(r"add\s+(.+?)\s+to\s+(?:my\s+)?shopping list", text, re.IGNORECASE)
        item = item_match.group(1).strip() if item_match else "milk"
        base.update(
            action_class="internal_action",
            side_effect_class="write",
            scenario_hint="shopping_list_assemble",
            slots={"item": item},
        )
    elif "prepared artifact" in normalized:
        artifact_match = re.search(r"prepared artifact\s+([a-zA-Z0-9.-]+)", text, re.IGNORECASE)
        artifact_id = artifact_match.group(1) if artifact_match else ""
        base.update(
            action_class="state_mutation",
            side_effect_class="write",
            scenario_hint="annotation_classify",
            slots={
                "artifact_id": artifact_id,
                "interaction_type": "made_it",
                "rating": 4,
                "note": "needs more garlic",
            },
        )
    elif "weather" in normalized:
        base.update(
            action_class="external_lookup",
            side_effect_class="external_read",
            scenario_hint="weather_query",
            tool_hints=["location_normalize", "weather_lookup"],
        )
    else:
        base.update(action_class="capture_only", side_effect_class="none")
    return base


class Handler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path != "/healthz":
            self.send_error(404)
            return
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"ok\n")

    def do_POST(self) -> None:
        if self.path != "/api/chat":
            self.send_error(404)
            return
        length = int(self.headers.get("Content-Length", "0"))
        request = json.loads(self.rfile.read(length))
        prompt = json.loads(request["messages"][-1]["content"])
        intent = compiled_intent(prompt["raw_turn"]["text"])
        response = {
            "model": request["model"],
            "message": {"role": "assistant", "content": json.dumps(intent, separators=(",", ":"))},
            "done": True,
        }
        body = json.dumps(response, separators=(",", ":")).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, _format: str, *_args: object) -> None:
        return


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8082), Handler).serve_forever()