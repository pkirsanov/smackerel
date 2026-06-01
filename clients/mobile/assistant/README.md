# Smackerel Assistant — Shared Flutter Skeleton (spec 073 Scope 1d)

This Flutter package is the single shared codebase that feeds both the iPhone/iOS and Android assistant clients. It contains:

- `lib/core/` — pure-Dart shared core: generated wire-schema models, render descriptors, turn-state machine. **No platform secure-storage, no shared-preferences, no filesystem use is permitted here** (enforced by `test/core_storage_guard_test.dart`).
- `lib/platform/ios/` and `lib/platform/android/` — adapter trees with matching platform-channel method signatures for secure session handoff (enforced by `test/platform_declaration_test.dart`).
- `lib/src/codegen.dart` — pure-Dart code generator that materialises `lib/core/generated/assistant_turn_v1.dart` from the canonical spec 069 wire-schema artifact at `internal/assistant/schema/assistant_turn_v1.json`.
- `tool/gen_dart_models.dart` — CLI entrypoint that regenerates the shared models when the canonical schema changes.

## NO-DEFAULTS

This package consumes the SST-derived backend base URL via the eight `mobile.assistant.*` keys added in Scope 1b. The Dart side MUST NOT supply a fallback hostname or port; missing configuration MUST fail loud at initialization.

## Linux CI scope

Per `design.md` → `Decision: iOS Native Build Verification Scope`, this repo's CI runs only:

```bash
dart analyze
flutter test
flutter build apk
```

`flutter build ios` is delegated to the deploy-overlay/macOS operator and is intentionally NOT run by this repo's CI.

## Regenerating models

```bash
cd clients/mobile/assistant
dart run tool/gen_dart_models.dart
```

The drift test (`test/codegen_drift_test.dart`) regenerates into a temporary directory and fails if the committed artifact diverges.
