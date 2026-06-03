// TP-076-07a-03 — Fail-loud config check: missing SMACKEREL_API_BASE_URL
// aborts the assistant shared-core build/start path (spec 076 SCOPE-7a;
// SCN-073-A11).
//
// The shared mobile core MUST refuse to start without a backend base
// URL. `AssistantConfig.loadFromEnv(env)` is the single start-time
// loader; absent or blank `SMACKEREL_API_BASE_URL` MUST throw a
// `StateError` that names the missing key so build pipelines and
// platform adapters surface it verbatim.

import 'package:flutter_test/flutter_test.dart';
import 'package:smackerel_assistant/core/config.dart';

void main() {
  group('TP-076-07a-03 — ConfigFailLoud_MissingBaseUrl', () {
    test('empty env throws StateError naming SMACKEREL_API_BASE_URL', () {
      expect(
        () => AssistantConfig.loadFromEnv(const <String, String>{}),
        throwsA(isA<StateError>().having(
          (StateError e) => e.message,
          'message',
          contains(smackerelApiBaseUrlEnvKey),
        )),
      );
    });

    test(
        'blank-string env throws StateError naming '
        'SMACKEREL_API_BASE_URL', () {
      for (final String blank in const <String>['', '   ', '\t\n']) {
        expect(
          () => AssistantConfig.loadFromEnv(
              <String, String>{smackerelApiBaseUrlEnvKey: blank}),
          throwsA(isA<StateError>().having(
            (StateError e) => e.message,
            'message',
            contains(smackerelApiBaseUrlEnvKey),
          )),
          reason: 'blank value `${blank.replaceAll('\n', '\\n')}` must fail',
        );
      }
    });

    test('typo\'d env key still fails loud (no silent default)', () {
      // Adversarial: prove a build-pipeline typo (`SMACKEREL_API` instead
      // of `SMACKEREL_API_BASE_URL`) does NOT get silently rescued.
      expect(
        () => AssistantConfig.loadFromEnv(
            const <String, String>{'SMACKEREL_API': 'https://example.com'}),
        throwsA(isA<StateError>().having(
          (StateError e) => e.message,
          'message',
          contains(smackerelApiBaseUrlEnvKey),
        )),
      );
    });

    test('present non-empty env returns config with trimmed base URL', () {
      final AssistantConfig cfg =
          AssistantConfig.loadFromEnv(const <String, String>{
        smackerelApiBaseUrlEnvKey: '  https://api.example.com  '
      });
      expect(cfg.apiBaseUrl, equals('https://api.example.com'));
    });
  });
}
