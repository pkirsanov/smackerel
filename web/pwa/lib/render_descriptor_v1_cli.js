#!/usr/bin/env node
// CLI wrapper for render_descriptor_v1: reads an assistant_turn_v1
// response JSON object from stdin, writes the render-descriptor-v1
// JSON to stdout. Used by tests/unit/clients/render_descriptor_canary_test.go.
'use strict';

const { renderToDescriptorV1 } = require('./render_descriptor_v1.js');

let raw = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', (chunk) => {
  raw += chunk;
});
process.stdin.on('end', () => {
  try {
    const input = JSON.parse(raw);
    const out = renderToDescriptorV1(input);
    process.stdout.write(JSON.stringify(out));
  } catch (err) {
    process.stderr.write(String((err && err.message) ? err.message : err));
    process.exit(2);
  }
});
