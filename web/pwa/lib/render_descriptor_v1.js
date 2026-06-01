// Pure JS renderer that projects a spec 069 assistant_turn_v1 response
// into the spec 073 render-descriptor-v1 shape.
//
// The renderer is intentionally branch-only on response shape, never on
// scenario id, action class, platform, or transport_hint. Output ordering
// is canonical: body text, citations (source order), disambiguation choices
// (choice order), confirm accept/decline pair, retry action.
//
// This module is the canonical JS reference consumed by:
//   - the planned web /assistant client renderer (spec 073 SCOPE-2)
//   - the cross-language renderer canary at
//     tests/unit/clients/render_descriptor_canary_test.go (TP-073-03)
'use strict';

const DESCRIPTOR_SCHEMA_VERSION = 'render-descriptor.v1';
const RESPONSE_SCHEMA_VERSION = 'v1';

function renderToDescriptorV1(response) {
  if (response === null || typeof response !== 'object' || Array.isArray(response)) {
    throw new Error('render_descriptor_v1: response must be an object');
  }
  if (response.schema_version !== RESPONSE_SCHEMA_VERSION) {
    throw new Error(
      'render_descriptor_v1: unsupported response schema_version: ' +
        String(response.schema_version)
    );
  }
  const nodes = [];

  const body = typeof response.body === 'string' ? response.body : '';
  if (body.length > 0) {
    nodes.push({ kind: 'text', text: body });
  }

  const sources = Array.isArray(response.sources) ? response.sources : [];
  for (const s of sources) {
    if (s === null || typeof s !== 'object') continue;
    const node = {
      kind: 'citation',
      source_id: String(s.id == null ? '' : s.id),
      label: String(s.title == null ? '' : s.title),
    };
    if (typeof s.url === 'string' && s.url.length > 0) {
      node.url = s.url;
    }
    nodes.push(node);
  }

  const dis = response.disambiguation_prompt;
  if (dis !== null && typeof dis === 'object' && !Array.isArray(dis)) {
    const ref = String(dis.disambiguation_ref == null ? '' : dis.disambiguation_ref);
    const choices = Array.isArray(dis.choices) ? dis.choices : [];
    for (const c of choices) {
      if (c === null || typeof c !== 'object') continue;
      const idx = typeof c.number === 'number' ? c.number : 0;
      nodes.push({
        kind: 'action',
        action_kind: 'disambiguation_choice',
        ref: ref,
        label: String(c.label == null ? '' : c.label),
        choice_index: idx,
      });
    }
  }

  const conf = response.confirm_card;
  if (conf !== null && typeof conf === 'object' && !Array.isArray(conf)) {
    const ref = String(conf.confirm_ref == null ? '' : conf.confirm_ref);
    nodes.push({
      kind: 'action',
      action_kind: 'confirm_accept',
      ref: ref,
      label: String(conf.positive_label == null ? '' : conf.positive_label),
    });
    nodes.push({
      kind: 'action',
      action_kind: 'confirm_decline',
      ref: ref,
      label: String(conf.negative_label == null ? '' : conf.negative_label),
    });
  }

  const err = typeof response.error_cause === 'string' ? response.error_cause : '';
  if (err.length > 0) {
    nodes.push({
      kind: 'action',
      action_kind: 'retry',
      ref: err,
      label: 'Retry',
    });
  }

  // Spec 075 SCOPE-075-06.3 — optional legacy-retirement notice rendered
  // as a one-line text addendum AFTER the primary body so it never
  // blocks the assistant response. Field is additive and optional on
  // the v1 wire contract; absent on every turn that did not match a
  // retired command.
  const notice = response.notice;
  if (notice !== null && typeof notice === 'object' && !Array.isArray(notice)) {
    const replacement = typeof notice.replacement_example === 'string' ? notice.replacement_example : '';
    if (replacement.length > 0) {
      nodes.push({ kind: 'text', text: replacement });
    }
  }

  return {
    schema_version: DESCRIPTOR_SCHEMA_VERSION,
    nodes: nodes,
  };
}

module.exports = { renderToDescriptorV1, DESCRIPTOR_SCHEMA_VERSION };
