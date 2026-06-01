# User Validation: 072 WhatsApp Business Webhook Adapter

## Checklist

- [x] Planning baseline reflects `spec.md` and `design.md` scenarios SCN-072-A01 through SCN-072-A10.
- [ ] WhatsApp text and interactive turns feel like the same assistant behavior as Telegram and HTTP.
- [ ] Unsigned webhook attempts are operator-visible and never produce user-facing assistant actions.
- [ ] Capture-as-fallback acknowledgement matches the canonical saved-as-idea shape.
- [ ] Operator can distinguish disabled, credential-error, signature-rejection, retry, and send-failure states.

## Planning Note

This checklist is a user-acceptance scaffold. Items other than the planning baseline require implementation and validation evidence before a human reviewer marks them complete.