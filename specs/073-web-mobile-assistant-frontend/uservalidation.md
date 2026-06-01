# User Validation: 073 Web/Mobile Assistant Frontend Client

## Checklist

- [x] Planning baseline reflects `spec.md` and `design.md` scenarios SCN-073-A01 through SCN-073-A11.
- [ ] Web user can submit a natural-language turn and understand the rendered response without command syntax.
- [ ] iPhone/iOS user gets equivalent disambiguation, confirmation, source, retry, and capture acknowledgement behavior.
- [ ] Android user gets equivalent disambiguation, confirmation, source, retry, and capture acknowledgement behavior.
- [ ] Shared mobile parity holds: iPhone/iOS and Android ship from one shared mobile codebase and expose the same assistant behavior.
- [ ] Web/mobile parity holds: the same `AssistantResponse` produces equivalent visible choices and actions on web, iPhone/iOS, and Android.
- [ ] Keyboard-only web use, iPhone/iOS VoiceOver, and Android TalkBack can reach composer, choices, confirms, citations, saved-as-idea acknowledgements, errors, and retry controls.
- [ ] Retry is idempotent on web, iPhone/iOS, and Android: retry preserves the original `transport_message_id` and does not duplicate side effects.
- [ ] Privacy/security reviewer can confirm clients do not persist sensitive bearer/session material in forbidden client storage.

## Planning Note

This checklist is a user-acceptance scaffold. Items other than the planning baseline require implementation and validation evidence before a human reviewer marks them complete.