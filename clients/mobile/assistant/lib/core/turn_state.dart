// Shared turn-state machine (spec 073 Scope 1d). Tracks the in-flight turn
// id used for idempotent retries. Pure-Dart; MUST NOT touch platform
// secure-storage or shared-preferences.

enum TurnPhase { idle, sending, awaitingResponse, complete, failed }

class TurnState {
  TurnState({required this.transportMessageId, this.phase = TurnPhase.idle});

  final String transportMessageId;
  TurnPhase phase;

  void markSending() => phase = TurnPhase.sending;
  void markAwaiting() => phase = TurnPhase.awaitingResponse;
  void markComplete() => phase = TurnPhase.complete;
  void markFailed() => phase = TurnPhase.failed;

  bool get isRetryable => phase == TurnPhase.failed;
}
