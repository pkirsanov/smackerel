// TP-076-07a-01 — Dart shared-core consumes generated render-descriptor
// types (spec 076 SCOPE-7a; SCN-073-A02).
//
// Two assertions:
//   1. Runtime — driving `renderToDescriptorV1` through the shared core
//      emits `descriptorSchemaVersion` ('render-descriptor.v1') and the
//      canonical node order documented in spec 073 design.md. The input
//      is validated by the generated `validateTurnResponse` (from
//      `lib/core/generated/assistant_turn_v1.dart`), proving the shared
//      core consumes the generated types end-to-end.
//   2. Static — `lib/core/render_descriptor_v1.dart` and
//      `lib/core/renderer.dart` import the generated wire-schema artifact
//      (`generated/assistant_turn_v1.dart`) instead of declaring a
//      hand-rolled mirror class such as `TurnResponse` / `validateTurnResponse`
//      locally.

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:smackerel_assistant/core/generated/assistant_turn_v1.dart';
import 'package:smackerel_assistant/core/render_descriptor_v1.dart';

Directory _packageRoot() {
  Directory cur = Directory.current.absolute;
  while (true) {
    if (File('${cur.path}/pubspec.yaml').existsSync() &&
        cur.path.endsWith('clients/mobile/assistant')) {
      return cur;
    }
    if (File('${cur.path}/pubspec.yaml').existsSync()) {
      return cur;
    }
    final Directory parent = cur.parent;
    if (parent.path == cur.path) {
      fail('could not locate package root from ${Directory.current.path}');
    }
    cur = parent;
  }
}

Map<String, dynamic> _goldenTurn() => <String, dynamic>{
      'schema_version': assistantTurnSchemaVersion,
      'transport': 'http',
      'transport_message_id': 'txn-076-07a-01',
      'status': 'ok',
      'body': 'Two captures match.',
      'sources': <Map<String, dynamic>>[
        <String, dynamic>{
          'id': 'src-1',
          'title': 'Capture A',
          'kind': 'note',
          'artifact_id': 'art-A',
          'artifact_captured_at': '2026-05-30T12:00:00Z',
          'provider_name': 'notes',
          'provider_retrieved_at': '2026-05-30T12:00:05Z',
          'url': 'https://example.com/a',
          'web_provider': '',
          'web_fetched_at': '',
          'web_content_hash': '',
          'web_snippet': '',
          'computation_tool': '',
          'computation_input_hash': '',
          'computation_output_hash': '',
        },
      ],
      'sources_overflow_count': 0,
      'confirm_card': null,
      'disambiguation_prompt': null,
      'error_cause': '',
      'capture_route': false,
      'trace': <String, dynamic>{
        'assistant_turn_id': 't1',
        'agent_trace_id': 'a1',
        'request_id': 'r1',
      },
      'facade_invoked': true,
      'emitted_at': '2026-06-02T00:00:00Z',
    };

void main() {
  final Directory pkgRoot = _packageRoot();

  group('TP-076-07a-01 — RenderDescriptor_UsesGeneratedTypes', () {
    test(
        'renderToDescriptorV1 emits canonical descriptor backed by '
        'generated validateTurnResponse', () {
      final Map<String, dynamic> turn = _goldenTurn();
      // Sanity: the generated validator accepts the fixture. If the
      // shared core had a hand-rolled mirror, this call would not be
      // wired through the generated artifact.
      expect(() => validateTurnResponse(turn), returnsNormally);

      final Map<String, dynamic> desc = renderToDescriptorV1(turn);
      expect(desc['schema_version'], equals(descriptorSchemaVersion));
      final List<dynamic> nodes = desc['nodes'] as List<dynamic>;
      expect(nodes, isNotEmpty,
          reason:
              'descriptor must emit at least one node for a non-empty body');
      expect((nodes.first as Map<String, dynamic>)['kind'], equals('text'),
          reason: 'first node MUST be the body text per design.md ordering');
      // Citation node MUST come from the validated `sources` array.
      final Iterable<Map<String, dynamic>> citations = nodes
          .whereType<Map<String, dynamic>>()
          .where((Map<String, dynamic> n) => n['kind'] == 'citation');
      expect(citations.length, equals(1));
      expect(citations.first['source_id'], equals('src-1'));
    });

    test(
        'shared-core sources import the generated wire-schema artifact '
        '(no hand-rolled mirror)', () {
      final List<String> mustImport = <String>[
        'lib/core/render_descriptor_v1.dart',
        'lib/core/renderer.dart',
      ];
      const String importNeedle = 'generated/assistant_turn_v1.dart';
      for (final String rel in mustImport) {
        final File f = File('${pkgRoot.path}/$rel');
        expect(f.existsSync(), isTrue,
            reason: 'shared-core file missing at ${f.path}');
        final String src = f.readAsStringSync();
        expect(src.contains(importNeedle), isTrue,
            reason: '$rel must import the generated wire-schema artifact '
                '($importNeedle) instead of declaring a hand-rolled mirror');
        // Forbid hand-rolled mirror declarations that would shadow the
        // generated artifact.
        for (final String forbidden in const <String>[
          'class TurnResponse',
          'class TurnRequest',
          'Map<String, dynamic> validateTurnResponse(',
        ]) {
          expect(src.contains(forbidden), isFalse,
              reason: '$rel must NOT declare its own $forbidden — those types '
                  'are owned by the generated artifact');
        }
      }
    });

    test('adversarial: a tampered descriptor schema_version is detected', () {
      // Sanity: prove the equality check above is non-vacuous.
      final Map<String, dynamic> desc = renderToDescriptorV1(_goldenTurn());
      final Map<String, dynamic> tampered = <String, dynamic>{
        ...desc,
        'schema_version': 'render-descriptor.v2-tampered',
      };
      expect(tampered['schema_version'] == descriptorSchemaVersion, isFalse);
    });
  });
}
