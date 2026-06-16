# User Validation — Spec 094 (Weather Rich Conditions + 10-Day Forecast)

**Report:** [report.md](report.md) · **Scopes:** [scopes.md](scopes.md) · **Spec:** [spec.md](spec.md)

> Items are **checked `[x]` by default** (the expected validated behavior). The operator **unchecks `[ ]`** to report a behavior that is broken — an unchecked item is a BLOCKING user-reported regression for `/bubbles.validate` to investigate. Evidence for each item lands under the matching [report.md](report.md) anchor at implement/test time.

## Checklist

- [x] When I ask for the weather, the answer shows **all** the conditions — condition, temperature, feels-like, humidity, precipitation, wind (speed + direction), UV, and sunrise/sunset — not just temperature. (AC-1)
- [x] The same answer includes a **10-day forecast**: one line per day with the condition, high/low temperature, chance of rain, and UV. (AC-2)
- [x] The forecast is **skimmable on my phone** — a short current block then one compact line per day; it does not overflow or look garbled in Telegram. (AC-5)
- [x] I can change how many days are shown (e.g. to 7) from config, and a bad value (or a missing one) makes the app refuse to start rather than guessing. (AC-3)
- [x] The unit of temperature, wind, and rain comes from config; a missing or wrong unit makes the app refuse to start rather than silently picking one. (AC-4)
- [x] Every answer still says **which provider** it came from and **when** it was fetched. (AC-7)
- [x] Asking the same place twice in a short window returns the same "as of" time (it does not pretend the cached answer is newer than it is). (AC-8)
- [x] If the weather provider is down, I get the existing "can't reach the provider" handling — never made-up numbers. (AC-9)
- [x] If I don't say where, I get the existing "which location?" handling, exactly as before. (AC-10)
- [x] Older phrasings still work — "weather in Barcelona tomorrow", "this weekend", "right now" — and now return the richer answer. (AC-11)
