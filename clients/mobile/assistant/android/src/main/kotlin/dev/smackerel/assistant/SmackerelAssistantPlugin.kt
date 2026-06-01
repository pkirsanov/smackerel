package dev.smackerel.assistant

import io.flutter.embedding.engine.plugins.FlutterPlugin

// Stub Android plugin implementation for spec 073 Scope 1d.
// The shared assistant core is pure Dart; this class exists only so that
// `flutter build apk` resolves the plugin declaration in pubspec.yaml.
// Real secure-session handoff is delegated to the deploy overlay per
// design.md → "Decision: iOS Native Build Verification Scope".
class SmackerelAssistantPlugin : FlutterPlugin {
    override fun onAttachedToEngine(binding: FlutterPlugin.FlutterPluginBinding) {
        // no-op
    }

    override fun onDetachedFromEngine(binding: FlutterPlugin.FlutterPluginBinding) {
        // no-op
    }
}
