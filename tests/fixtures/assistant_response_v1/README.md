# Assistant Response v1 — Shared Renderer Fixtures (spec 073, TP-073-03)

Each scenario in this directory consists of a paired input/golden file:

- `<name>.input.json` — spec 069 `assistant_turn_v1` response payload.
- `<name>.descriptor.json` — golden `render-descriptor-v1` produced by the
  shared renderer.

Both the JavaScript renderer (`web/pwa/lib/render_descriptor_v1.js`) and the
Dart renderer (`clients/mobile/assistant/lib/core/render_descriptor_v1.dart`)
MUST emit, for the same input, an object that JSON-deep-equals the paired
golden after validation against [`render-descriptor-v1.json`](./render-descriptor-v1.json).

The Go canary test at `tests/unit/clients/render_descriptor_canary_test.go`
shells out to `node` and `dart` and enforces this contract.
