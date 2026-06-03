// Fail-loud runtime configuration for the assistant shared core
// (spec 076 SCOPE-7a; SCN-073-A11).
//
// The shared mobile core MUST NOT silently default the backend base URL.
// `AssistantConfig.loadFromEnv(env)` is the single start-time loader:
// callers pass `Platform.environment` (or the platform-channel-provided
// equivalent) and any missing or blank `SMACKEREL_API_BASE_URL` causes a
// StateError that names the required key. There is no fallback default,
// no localhost shim, and no silent skip — the build/start path fails
// loud the moment it is invoked.
//
// This file lives in `lib/core/` and is pure Dart: it MUST NOT import
// any platform secure-storage, shared-preferences, file-system, or
// path-provider API (see TP-073-26 storage guard).

/// Required environment-variable key for the backend base URL. Mobile
/// build pipelines (iOS scheme env, Android Gradle env, Flutter
/// `--dart-define`) MUST inject this key; otherwise startup fails loud
/// via `AssistantConfig.loadFromEnv`.
const String smackerelApiBaseUrlEnvKey = 'SMACKEREL_API_BASE_URL';

/// Runtime configuration consumed by the assistant shared core.
class AssistantConfig {
  const AssistantConfig({required this.apiBaseUrl});

  /// Backend base URL (no trailing slash). Always non-empty after a
  /// successful `loadFromEnv`.
  final String apiBaseUrl;

  /// Load configuration from the supplied environment map. Throws
  /// [StateError] naming the missing key when `SMACKEREL_API_BASE_URL`
  /// is absent or blank — the build/start path is expected to surface
  /// this error verbatim. No defaulting, no fallback.
  static AssistantConfig loadFromEnv(Map<String, String> env) {
    final String? raw = env[smackerelApiBaseUrlEnvKey];
    if (raw == null || raw.trim().isEmpty) {
      throw StateError('missing required environment variable '
          '$smackerelApiBaseUrlEnvKey (assistant shared core cannot '
          'start without a backend base URL; build/start aborted)');
    }
    return AssistantConfig(apiBaseUrl: raw.trim());
  }
}
