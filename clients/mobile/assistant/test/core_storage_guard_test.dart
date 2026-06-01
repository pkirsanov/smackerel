// TP-073-26 — shared mobile core storage guard (spec 073 Scope 1d).
//
// Static scan of `lib/core/` that forbids any reference to platform
// secure-storage, shared-preferences, file-system, or path_provider APIs
// that could persist bearer/session material outside the platform adapter
// trees. Only `lib/platform/ios/` and `lib/platform/android/` are allowed
// to touch secure session handoff.
//
// SCN-073-A11: "Shared mobile renderer/core does not persist bearer/session
// material — no shared-core symbol calls platform secure-storage, file
// system, or shared preferences with bearer/session material."

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

/// Forbidden API tokens. Any substring match in a `lib/core/` source file
/// fails the guard.
const List<String> _forbidden = <String>[
  'flutter_secure_storage',
  'FlutterSecureStorage',
  'shared_preferences',
  'SharedPreferences',
  'path_provider',
  'getApplicationDocumentsDirectory',
  'getApplicationSupportDirectory',
  'dart:io',
  'package:hive',
  'package:sqflite',
  'package:isar',
];

List<File> _coreSources(Directory pkgRoot) {
  final Directory core = Directory('${pkgRoot.path}/lib/core');
  if (!core.existsSync()) {
    return <File>[];
  }
  return core
      .listSync(recursive: true)
      .whereType<File>()
      .where((File f) => f.path.endsWith('.dart'))
      .toList();
}

void main() {
  final Directory pkgRoot = _packageRoot();

  group('TP-073-26 — shared mobile core storage guard', () {
    test('lib/core/ contains no forbidden persistence API references', () {
      final List<File> files = _coreSources(pkgRoot);
      expect(files, isNotEmpty,
          reason: 'lib/core/ has no Dart sources — scan would be vacuous');
      final List<String> violations = <String>[];
      for (final File f in files) {
        final String src = f.readAsStringSync();
        for (final String token in _forbidden) {
          if (src.contains(token)) {
            violations.add('${f.path}: contains forbidden token "$token"');
          }
        }
      }
      expect(violations, isEmpty,
          reason: 'shared core MUST NOT touch persistence APIs:\n'
              '${violations.join('\n')}');
    });

    test('adversarial: guard would flag a planted violation', () {
      // RED-style sanity: prove the substring scan is meaningful by
      // exercising it against a synthetic source blob.
      const String plantedSource = "import 'package:flutter_secure_storage/x';";
      bool detected = false;
      for (final String token in _forbidden) {
        if (plantedSource.contains(token)) {
          detected = true;
          break;
        }
      }
      expect(detected, isTrue,
          reason: 'storage guard scan must detect known forbidden tokens');
    });
  });
}
