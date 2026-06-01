// CLI wrapper for render_descriptor_v1: reads a spec 069 assistant_turn_v1
// response JSON object from stdin, writes the canonical
// render-descriptor-v1 JSON to stdout. Used by the cross-language
// renderer canary at
// `tests/unit/clients/render_descriptor_canary_test.go` (TP-073-03).
//
// Run from the package root:
//   dart run tool/render_descriptor_v1_cli.dart < input.json

import 'dart:convert';
import 'dart:io';

import 'package:smackerel_assistant/core/render_descriptor_v1.dart';

Future<void> main(List<String> args) async {
  final String raw = await utf8.decoder.bind(stdin).join();
  try {
    final Object? decoded = jsonDecode(raw);
    if (decoded is! Map<String, dynamic>) {
      stderr.write('render_descriptor_v1_cli: stdin must be a JSON object');
      exitCode = 2;
      return;
    }
    final Map<String, dynamic> descriptor = renderToDescriptorV1(decoded);
    stdout.write(jsonEncode(descriptor));
  } catch (e) {
    stderr.write(e.toString());
    exitCode = 2;
  }
}
