// Android platform adapter stub for the Smackerel assistant secure session
// channel (spec 073 Scope 1d). Mirrors the iOS counterpart in
// `../ios/secure_session_channel.dart`. The `SecureSessionChannel`
// signature MUST match across platforms; this is enforced by
// `test/platform_declaration_test.dart`.

import 'dart:async';

abstract class SecureSessionChannel {
  /// Hand a bearer/session token to the platform's secure store and return
  /// a handle the shared core can carry without persisting the secret.
  Future<String> exchangeSessionHandshake(String bearer);

  /// Clear any persisted session material.
  Future<void> clearSession();
}

/// Android implementation stub. Concrete `MethodChannel` wiring is supplied
/// by the Android host plugin in the deploy-overlay; the stub here exists
/// so the shared codebase compiles and the platform-declaration test can
/// assert matching method signatures across `ios/` and `android/`.
class AndroidSecureSessionChannel implements SecureSessionChannel {
  const AndroidSecureSessionChannel();

  @override
  Future<String> exchangeSessionHandshake(String bearer) async {
    throw UnimplementedError(
        'AndroidSecureSessionChannel.exchangeSessionHandshake must be wired by the Android host plugin');
  }

  @override
  Future<void> clearSession() async {
    throw UnimplementedError(
        'AndroidSecureSessionChannel.clearSession must be wired by the Android host plugin');
  }
}
