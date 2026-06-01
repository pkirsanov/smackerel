// Dart projection that maps a spec 069 assistant_turn_v1 response into the
// canonical spec 073 render-descriptor-v1 shape (see
// `specs/073-web-mobile-assistant-frontend/design.md` § Render-Descriptor
// JSON Schema). Output ordering is canonical and identical to the JS
// reference at `web/pwa/lib/render_descriptor_v1.js`: body text, citations
// (source order), disambiguation choices (choice order), confirm
// accept/decline pair, retry action.
//
// This file is the shared core for both the iOS and Android adapter
// projections used by the spec 073 cross-language renderer canary
// (TP-073-03). It MUST NOT import any platform secure-storage, shared
// preferences, file-system, or path-provider API.

import 'generated/assistant_turn_v1.dart';

const String descriptorSchemaVersion = 'render-descriptor.v1';

/// Project a validated assistant turn response into a JSON-serializable
/// render-descriptor-v1 map. The input is validated via
/// `validateTurnResponse`; any unsupported response `schema_version`
/// throws before any node is emitted.
Map<String, dynamic> renderToDescriptorV1(Map<String, dynamic> response) {
  validateTurnResponse(response);

  final List<Map<String, dynamic>> nodes = <Map<String, dynamic>>[];

  final String body = response['body'] as String;
  if (body.isNotEmpty) {
    nodes.add(<String, dynamic>{'kind': 'text', 'text': body});
  }

  final List<dynamic> sources = response['sources'] as List<dynamic>;
  for (final dynamic raw in sources) {
    if (raw is! Map<String, dynamic>) continue;
    final Map<String, dynamic> node = <String, dynamic>{
      'kind': 'citation',
      'source_id': (raw['id'] ?? '').toString(),
      'label': (raw['title'] ?? '').toString(),
    };
    final dynamic url = raw['url'];
    if (url is String && url.isNotEmpty) {
      node['url'] = url;
    }
    nodes.add(node);
  }

  final dynamic dis = response['disambiguation_prompt'];
  if (dis is Map<String, dynamic>) {
    final String ref = (dis['disambiguation_ref'] ?? '').toString();
    final List<dynamic> choices = (dis['choices'] as List<dynamic>?) ?? const <dynamic>[];
    for (final dynamic c in choices) {
      if (c is! Map<String, dynamic>) continue;
      final dynamic number = c['number'];
      final int idx = number is int ? number : 0;
      nodes.add(<String, dynamic>{
        'kind': 'action',
        'action_kind': 'disambiguation_choice',
        'ref': ref,
        'label': (c['label'] ?? '').toString(),
        'choice_index': idx,
      });
    }
  }

  final dynamic confirm = response['confirm_card'];
  if (confirm is Map<String, dynamic>) {
    final String ref = (confirm['confirm_ref'] ?? '').toString();
    nodes.add(<String, dynamic>{
      'kind': 'action',
      'action_kind': 'confirm_accept',
      'ref': ref,
      'label': (confirm['positive_label'] ?? '').toString(),
    });
    nodes.add(<String, dynamic>{
      'kind': 'action',
      'action_kind': 'confirm_decline',
      'ref': ref,
      'label': (confirm['negative_label'] ?? '').toString(),
    });
  }

  final String errCause = response['error_cause'] as String;
  if (errCause.isNotEmpty) {
    nodes.add(<String, dynamic>{
      'kind': 'action',
      'action_kind': 'retry',
      'ref': errCause,
      'label': 'Retry',
    });
  }

  return <String, dynamic>{
    'schema_version': descriptorSchemaVersion,
    'nodes': nodes,
  };
}
