// Pure-Dart render descriptors and renderer for the assistant turn response
// (spec 073 Scope 1d). This file is the shared core consumed by both the
// iOS and Android platform adapters. It MUST NOT import any platform
// secure storage, shared preferences, file system, or path provider API.
//
// The renderer takes a `TurnResponse`-shaped JSON map (already validated by
// `validateTurnResponse` in generated/assistant_turn_v1.dart) and emits a
// platform-agnostic list of `RenderDescriptor`s. The output depends only on
// the response shape; the platform target is informational metadata only.

import 'generated/assistant_turn_v1.dart';

/// Target platform that an adapter declares when invoking the shared
/// renderer. The renderer MUST produce equivalent descriptors for both
/// targets given the same response.
enum PlatformTarget { ios, android }

/// Closed-vocabulary descriptor kinds the renderer emits. The platform
/// adapter is responsible for mapping each descriptor kind to a concrete
/// UI widget; descriptor data is shared across platforms.
enum RenderDescriptorKind {
  body,
  source,
  confirmCard,
  disambiguation,
  captureAcknowledgement,
  error,
  legacyRetirementNotice,
}

class RenderDescriptor {
  const RenderDescriptor({
    required this.kind,
    required this.fields,
  });

  final RenderDescriptorKind kind;
  final Map<String, Object?> fields;

  @override
  bool operator ==(Object other) {
    if (other is! RenderDescriptor) return false;
    if (other.kind != kind) return false;
    if (other.fields.length != fields.length) return false;
    for (final String k in fields.keys) {
      if (other.fields[k] != fields[k]) return false;
    }
    return true;
  }

  @override
  int get hashCode {
    int h = kind.hashCode;
    for (final String k in fields.keys.toList()..sort()) {
      h = 0x1fffffff & (h * 31 + k.hashCode);
      h = 0x1fffffff & (h * 31 + fields[k].hashCode);
    }
    return h;
  }

  @override
  String toString() => 'RenderDescriptor(${kind.name}, $fields)';
}

/// Render the validated turn-response map into a list of descriptors. The
/// `target` argument is recorded for telemetry only and MUST NOT influence
/// the descriptor list — same input ⇒ same output for any target.
List<RenderDescriptor> renderTurnResponse(
  Map<String, dynamic> response, {
  required PlatformTarget target,
}) {
  validateTurnResponse(response);
  final List<RenderDescriptor> out = <RenderDescriptor>[];
  final String body = response['body'] as String;
  if (body.isNotEmpty) {
    out.add(RenderDescriptor(
      kind: RenderDescriptorKind.body,
      fields: <String, Object?>{'text': body},
    ));
  }
  final List<dynamic> sources = response['sources'] as List<dynamic>;
  for (final dynamic raw in sources) {
    final Map<String, dynamic> s = raw as Map<String, dynamic>;
    out.add(RenderDescriptor(
      kind: RenderDescriptorKind.source,
      fields: <String, Object?>{
        'id': s['id'],
        'title': s['title'],
        'kind': s['kind'],
        'url': s['url'],
      },
    ));
  }
  final dynamic confirm = response['confirm_card'];
  if (confirm is Map<String, dynamic>) {
    out.add(RenderDescriptor(
      kind: RenderDescriptorKind.confirmCard,
      fields: <String, Object?>{
        'proposed_action': confirm['proposed_action'],
        'confirm_ref': confirm['confirm_ref'],
        'positive_label': confirm['positive_label'],
        'negative_label': confirm['negative_label'],
        'timeout_seconds': confirm['timeout_seconds'],
      },
    ));
  }
  final dynamic dis = response['disambiguation_prompt'];
  if (dis is Map<String, dynamic>) {
    out.add(RenderDescriptor(
      kind: RenderDescriptorKind.disambiguation,
      fields: <String, Object?>{
        'disambiguation_ref': dis['disambiguation_ref'],
        'timeout_seconds': dis['timeout_seconds'],
        'choices': (dis['choices'] as List<dynamic>)
            .map<Map<String, Object?>>((dynamic c) {
          final Map<String, dynamic> m = c as Map<String, dynamic>;
          return <String, Object?>{
            'number': m['number'],
            'id': m['id'],
            'label': m['label'],
            'shortcut': m['shortcut'],
          };
        }).toList(growable: false),
      },
    ));
  }
  if (response['capture_route'] == true) {
    out.add(const RenderDescriptor(
      kind: RenderDescriptorKind.captureAcknowledgement,
      fields: <String, Object?>{'captured': true},
    ));
  }
  final String errorCause = response['error_cause'] as String;
  if (errorCause.isNotEmpty) {
    out.add(RenderDescriptor(
      kind: RenderDescriptorKind.error,
      fields: <String, Object?>{'error_cause': errorCause},
    ));
  }
  // Spec 075 / 076 SCOPE-6c — legacy-retirement notice descriptor.
  // Server-side ledger owns dedup; the platform adapter renders this
  // descriptor as a one-line addendum AFTER the primary body. Same
  // server-emitted NoticePayload that WhatsApp + Telegram + PWA consume.
  final dynamic notice = response['notice'];
  if (notice is Map<String, dynamic>) {
    final dynamic cmdRaw = notice['command'];
    final dynamic exRaw = notice['replacement_example'];
    final String cmd = cmdRaw is String ? cmdRaw : '';
    final String ex = exRaw is String ? exRaw : '';
    if (cmd.isNotEmpty && ex.isNotEmpty) {
      out.add(RenderDescriptor(
        kind: RenderDescriptorKind.legacyRetirementNotice,
        fields: <String, Object?>{
          'command': cmd,
          'replacement_example': ex,
          'copy_key': notice['copy_key'],
          'window_id': notice['window_id'],
        },
      ));
    }
  }
  // target is intentionally not consulted; recorded only as a side-channel
  // assertion-friendly value via `lastRenderTarget` if callers need it.
  _lastRenderTarget = target;
  return List<RenderDescriptor>.unmodifiable(out);
}

PlatformTarget? _lastRenderTarget;
PlatformTarget? get lastRenderTarget => _lastRenderTarget;
