# User Validation: BUG-077-004 Photos PWA Cookie-Auth Assertion

## Checklist

- [x] The Photos connector wizard test matches the PWA's HttpOnly same-origin cookie authentication contract.
- [x] The regression fails if the wizard explicitly omits the session cookie.
- [x] No production authentication behavior is changed.