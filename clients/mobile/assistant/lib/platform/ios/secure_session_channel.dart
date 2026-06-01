// iOS platform adapter stub for the Smackerel assistant secure session
// channel (spec 073 Scope 1d). The shared `lib/core/` never touches
// secure-storage — bearer/session material flows only through this
// adapter, which delegates to a platform-channel implementation in the
// host iOS app/plugin.
//
// The Android counterpart in `../android/secure_session_channel.dart`
// declares the same `SecureSessionChannel` abstract interface, enforced
// by `test/platform_declaration_test.dart`.

import 'dart:async';

abstract class SecureSessionChannel {
  /// Hand a bearer/session token to the platform's secure store and return
  /// a handle the shared core can carry without persisting the secret.
  Future<String> exchangeSessionHandshake(String bearer);

  /// Clear any persisted session material.
  Future<void> clearSession();
}

/// iOS implementation stub. Concrete `MethodChannel` wiring is supplied by
/// the iOS host plugin in the deploy-overlay; the stub here exists so the
/// shared codebase compiles and the platform-declaration test can assert
/// matching method signatures across `ios/` and `android/`.
class IosSecureSessionChannel implements SecureSessionChannel {
  const IosSecureSessionChannel();

  @override
  Future<String> exchangeSessionHandshake(String bearer) async {
    throw UnimplementedError(
        'IosSecureSessionChannel.exchangeSessionHandshake must be wired by the iOS host plugin');
  }

  @override
  Future<void> clearSession() async {
    throw UnimplementedError(
        'IosSecureSessionChannel.clearSession must be wired by the iOS host plugin');
  }
}
