// TP-076-07a-02 — Static scan: shared mobile core has zero client-side
// scenario branching (spec 076 SCOPE-7a; SCN-073-A07).
//
// Walks every Dart source under `lib/core/` (excluding the generated
// artifact subtree `lib/core/generated/`) and fails if any file contains
// a forbidden scenario-branching token. Scenario classification is the
// server facade's job (see spec 064 / 065 / 066); the shared mobile core
// MUST treat the assistant response uniformly, dispatching only on the
// closed render-descriptor vocabulary already validated by the wire
// schema. Per-intent / per-tool / per-scenario `switch`/`if` branches
// in the client are exactly the regression this guard prevents.

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

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

/// Switch-discriminator identifiers that would betray client-side
/// scenario branching. The renderer only ever switches on render
/// descriptor kinds (an enum), never on these response-level strings.
const List<String> _forbiddenSwitchDiscriminators = <String>[
  'switch (intent',
  'switch (scenario',
  'switch (toolCall',
  'switch (tool_call',
  'switch (command',
  'switch (response[\'intent\']',
  'switch (response[\'scenario\']',
  'switch (response[\'tool_call\']',
];

/// Case-label literals from the documented intent / scenario vocabulary
/// (spec 064 open knowledge, spec 065 micro-tools, spec 066 legacy
/// retirement, spec 073 transport surfaces). If any of these appear as a
/// `case '...':` arm inside `lib/core/`, the client is branching on
/// scenario identity instead of letting the server decide.
const List<String> _forbiddenCaseLiterals = <String>[
  "case 'find':",
  "case 'rate':",
  "case 'replace':",
  "case 'capture':",
  "case 'idea':",
  "case 'web_research':",
  "case 'open_knowledge':",
  "case 'weather':",
  "case 'remind':",
  "case 'route':",
  "case 'reset':",
];

/// Equality / identity expressions that classify on scenario strings.
const List<String> _forbiddenEqualityExpressions = <String>[
  "intent == '",
  "scenario == '",
  "toolCall == '",
  "tool_call == '",
  "command == '",
  "response['intent']",
  "response['scenario']",
  "response['tool_call']",
];

List<File> _scanTargets(Directory pkgRoot) {
  final Directory core = Directory('${pkgRoot.path}/lib/core');
  if (!core.existsSync()) return <File>[];
  return core
      .listSync(recursive: true)
      .whereType<File>()
      .where((File f) =>
          f.path.endsWith('.dart') && !f.path.contains('/lib/core/generated/'))
      .toList();
}

void main() {
  final Directory pkgRoot = _packageRoot();

  group('TP-076-07a-02 — NoClientScenarioBranches_StaticScan', () {
    test('lib/core/ (non-generated) contains no scenario-branching tokens', () {
      final List<File> files = _scanTargets(pkgRoot);
      expect(files, isNotEmpty,
          reason: 'no scannable core sources — guard would be vacuous; the '
              'shared core has been deleted or relocated');

      final List<String> violations = <String>[];
      for (final File f in files) {
        final String src = f.readAsStringSync();
        for (final String needle in <String>[
          ..._forbiddenSwitchDiscriminators,
          ..._forbiddenCaseLiterals,
          ..._forbiddenEqualityExpressions,
        ]) {
          if (src.contains(needle)) {
            violations
                .add('${f.path}: contains forbidden token `${needle.trim()}`');
          }
        }
      }
      expect(violations, isEmpty,
          reason: 'client-side scenario branching detected:\n  '
              '${violations.join('\n  ')}');
    });

    test('adversarial: synthetic per-intent switch arm is detected', () {
      // Sanity: prove the substring check would catch a regression that
      // sprinkled per-intent client logic into the shared core.
      const String synthetic = "switch (intent) {\n  case 'find':\n  }";
      bool tripped = false;
      for (final String needle in <String>[
        ..._forbiddenSwitchDiscriminators,
        ..._forbiddenCaseLiterals,
      ]) {
        if (synthetic.contains(needle)) {
          tripped = true;
          break;
        }
      }
      expect(tripped, isTrue,
          reason: 'guard regex set must catch a synthetic per-intent switch');
    });
  });
}
