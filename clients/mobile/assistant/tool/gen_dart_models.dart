// Regenerates clients/mobile/assistant/lib/core/generated/assistant_turn_v1.dart
// from the canonical spec 069 wire-schema artifact.
//
// Usage (from repo root or from clients/mobile/assistant/):
//   cd clients/mobile/assistant && dart run tool/gen_dart_models.dart
//
// Spec 073 Scope 1d.

import 'dart:io';

import 'package:smackerel_assistant/src/codegen.dart';

void main(List<String> args) {
  final Directory repoRoot = _findRepoRoot(Directory.current);
  final File schema = File('${repoRoot.path}/$schemaFile');
  if (!schema.existsSync()) {
    stderr.writeln('schema artifact not found at ${schema.path}');
    exitCode = 2;
    return;
  }
  final String dartSource =
      generateAssistantTurnV1Dart(schema.readAsBytesSync());
  final File out = File('${repoRoot.path}/$generatedFile');
  out.parent.createSync(recursive: true);
  out.writeAsStringSync(dartSource);
  stdout.writeln('wrote ${out.path} (${dartSource.length} bytes)');
}

Directory _findRepoRoot(Directory start) {
  Directory cur = start.absolute;
  while (true) {
    if (Directory('${cur.path}/.git').existsSync() ||
        File('${cur.path}/go.mod').existsSync()) {
      return cur;
    }
    final Directory parent = cur.parent;
    if (parent.path == cur.path) {
      throw StateError(
          'could not locate repo root from ${start.absolute.path}');
    }
    cur = parent;
  }
}
