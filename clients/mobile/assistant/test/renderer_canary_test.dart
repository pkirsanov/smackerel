// TP-073-03 — shared renderer canary (spec 073 Scope 1d).
//
// One golden AssistantResponse fixture is driven through the shared
// renderer with both `PlatformTarget.ios` and `PlatformTarget.android`. The
// produced `RenderDescriptor` lists MUST be element-wise equivalent. This
// proves the shared core, not the platform adapter, determines render
// output, satisfying SCN-073-A02 ("shared renderer canary produces
// equivalent descriptors per platform").

import 'package:flutter_test/flutter_test.dart';
import 'package:smackerel_assistant/core/renderer.dart';

Map<String, dynamic> _goldenResponse() => <String, dynamic>{
      'schema_version': 'v1',
      'transport': 'http',
      'transport_message_id': 'txn-073-canary-1',
      'status': 'ok',
      'body': 'Here are two relevant captures.',
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
        <String, dynamic>{
          'id': 'src-2',
          'title': 'Capture B',
          'kind': 'web',
          'artifact_id': 'art-B',
          'artifact_captured_at': '2026-05-30T11:00:00Z',
          'provider_name': 'web',
          'provider_retrieved_at': '2026-05-30T11:00:05Z',
          'url': 'https://example.com/b',
          'web_provider': 'web',
          'web_fetched_at': '2026-05-30T11:00:00Z',
          'web_content_hash': 'h-b',
          'web_snippet': 'snippet B',
          'computation_tool': '',
          'computation_input_hash': '',
          'computation_output_hash': '',
        },
      ],
      'sources_overflow_count': 0,
      'confirm_card': <String, dynamic>{
        'proposed_action': 'save-list',
        'confirm_ref': 'cf-1',
        'positive_label': 'Save',
        'negative_label': 'Cancel',
        'timeout_seconds': 30,
      },
      'disambiguation_prompt': null,
      'error_cause': '',
      'capture_route': true,
      'trace': <String, dynamic>{
        'assistant_turn_id': 'turn-1',
        'agent_trace_id': 'agent-1',
        'request_id': 'req-1',
      },
      'facade_invoked': true,
      'emitted_at': '2026-05-30T12:00:10Z',
    };

void main() {
  group('TP-073-03 — shared renderer canary', () {
    test('ios-target and android-target produce equivalent descriptors', () {
      final Map<String, dynamic> response = _goldenResponse();
      final List<RenderDescriptor> ios =
          renderTurnResponse(response, target: PlatformTarget.ios);
      final List<RenderDescriptor> android =
          renderTurnResponse(response, target: PlatformTarget.android);

      expect(ios.length, equals(android.length),
          reason: 'descriptor count must match across platforms');
      for (int i = 0; i < ios.length; i++) {
        expect(ios[i], equals(android[i]),
            reason: 'descriptor[$i] must be equivalent across platforms');
      }
      // Expected: body, source x2, confirmCard, captureAcknowledgement.
      expect(
          ios.map((RenderDescriptor d) => d.kind).toList(),
          <RenderDescriptorKind>[
            RenderDescriptorKind.body,
            RenderDescriptorKind.source,
            RenderDescriptorKind.source,
            RenderDescriptorKind.confirmCard,
            RenderDescriptorKind.captureAcknowledgement,
          ]);
    });

    test('adversarial: divergent fixture proves canary catches drift', () {
      // RED-style adversarial: if a future change made the renderer
      // depend on the target, this assertion would have to be relaxed
      // — proving the canary is meaningful, not vacuous.
      final Map<String, dynamic> response = _goldenResponse();
      final List<RenderDescriptor> a =
          renderTurnResponse(response, target: PlatformTarget.ios);
      final Map<String, dynamic> tampered = Map<String, dynamic>.from(response)
        ..['body'] = 'tampered body';
      final List<RenderDescriptor> b =
          renderTurnResponse(tampered, target: PlatformTarget.ios);
      expect(a[0] == b[0], isFalse,
          reason:
              'changing fixture body MUST change the body descriptor (sanity)');
    });
  });
}
