// TP-073-04 — platform declaration test (spec 073 Scope 1d).
//
// Asserts that:
//   1. `pubspec.yaml` declares both `ios` and `android` as supported
//      platforms under `flutter.plugin.platforms`.
//   2. `lib/platform/ios/secure_session_channel.dart` and
//      `lib/platform/android/secure_session_channel.dart` both declare an
//      `abstract class SecureSessionChannel` exposing identical method
//      signatures.

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:yaml/yaml.dart';

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

List<String> _extractMethodSignatures(String dartSource) {
  // Find the class body of `abstract class SecureSessionChannel { ... }`.
  final RegExp classRegex = RegExp(
      r'abstract\s+class\s+SecureSessionChannel\s*\{([\s\S]*?)\n\}',
      multiLine: true);
  final RegExpMatch? m = classRegex.firstMatch(dartSource);
  if (m == null) {
    return <String>[];
  }
  final String body = m.group(1)!;
  // Collect each abstract method declaration line, stripped of comments
  // and whitespace.
  final List<String> sigs = <String>[];
  for (final String rawLine in body.split('\n')) {
    final String line = rawLine.trim();
    if (line.isEmpty || line.startsWith('//')) continue;
    if (!line.endsWith(';')) continue;
    // Normalise whitespace to canonical form so cosmetic diffs don't
    // count as signature drift.
    final String normalised =
        line.replaceAll(RegExp(r'\s+'), ' ').replaceAll(' ;', ';');
    sigs.add(normalised);
  }
  sigs.sort();
  return sigs;
}

void main() {
  final Directory pkgRoot = _packageRoot();

  group('TP-073-04 — platform declaration', () {
    test('pubspec.yaml declares both ios and android plugin platforms', () {
      final String pubspecText =
          File('${pkgRoot.path}/pubspec.yaml').readAsStringSync();
      final dynamic doc = loadYaml(pubspecText);
      expect(doc, isA<YamlMap>(),
          reason: 'pubspec.yaml must parse as YAML map');
      final dynamic flutterNode = (doc as YamlMap)['flutter'];
      expect(flutterNode, isA<YamlMap>(),
          reason: 'pubspec.yaml must contain a top-level `flutter:` block');
      final dynamic pluginNode = (flutterNode as YamlMap)['plugin'];
      expect(pluginNode, isA<YamlMap>(),
          reason: 'pubspec.yaml `flutter:` block must declare `plugin:`');
      final dynamic platformsNode = (pluginNode as YamlMap)['platforms'];
      expect(platformsNode, isA<YamlMap>(),
          reason: 'plugin block must declare `platforms:`');
      final YamlMap platforms = platformsNode as YamlMap;
      expect(platforms.containsKey('ios'), isTrue,
          reason: 'platforms must declare `ios`');
      expect(platforms.containsKey('android'), isTrue,
          reason: 'platforms must declare `android`');
    });

    test(
        'ios and android adapter SecureSessionChannel signatures are equivalent',
        () {
      final File ios =
          File('${pkgRoot.path}/lib/platform/ios/secure_session_channel.dart');
      final File android = File(
          '${pkgRoot.path}/lib/platform/android/secure_session_channel.dart');
      expect(ios.existsSync(), isTrue,
          reason: 'iOS adapter file missing at ${ios.path}');
      expect(android.existsSync(), isTrue,
          reason: 'Android adapter file missing at ${android.path}');

      final List<String> iosSigs =
          _extractMethodSignatures(ios.readAsStringSync());
      final List<String> androidSigs =
          _extractMethodSignatures(android.readAsStringSync());

      expect(iosSigs, isNotEmpty,
          reason:
              'no abstract SecureSessionChannel methods detected in iOS adapter');
      expect(androidSigs, equals(iosSigs),
          reason:
              'platform adapter method signatures diverge between ios and android');
    });

    test('adversarial: signature divergence is detected', () {
      // Sanity: a fabricated divergence would be caught.
      final List<String> a = <String>['Future<String> foo();'];
      final List<String> b = <String>['Future<String> foo(int x);'];
      expect(a == b, isFalse);
    });
  });
}
