# Recipe: Cross-Model Review

## When to Use

- Before shipping high-risk changes where a second AI opinion adds confidence
- When Claude's review missed something subtle and you want a different perspective
- For adversarial challenge on security-sensitive code
- When you want to compare findings across different AI architectures

## Setup (One-Time)

1. Configure your model registry in `.specify/memory/bubbles.config.json`:

```json
{
  "crossModelReview": {
    "enabled": true,
    "lastVerified": "2026-03-31",
    "autoTrackUsage": true,
    "models": [
      {
        "name": "claude-opus-4.6-1m",
        "provider": "anthropic",
        "role": "primary",
        "available": true,
        "notes": "1M token context, primary deep analysis model",
        "usageCount": 0,
        "lastUsed": null
      },
      {
        "name": "gpt-5.4",
        "provider": "openai",
        "role": "reviewer",
        "available": true,
        "notes": "Independent code review and adversarial challenge",
        "usageCount": 0,
        "lastUsed": null
      },
      {
        "name": "gpt-5.3-codex",
        "provider": "openai",
        "role": "reviewer",
        "available": true,
        "notes": "Codex variant, strong for code review",
        "usageCount": 0,
        "lastUsed": null
      },
      {
        "name": "gemini-3.1-pro",
        "provider": "google",
        "role": "adversarial",
        "available": true,
        "notes": "Occasional use for third-opinion adversarial review",
        "usageCount": 0,
        "lastUsed": null
      }
    ]
  }
}
```

2. Ensure the cross-model CLI tool (e.g., Codex CLI) is installed and accessible

## Dynamic Registry — Usage Tracking

The registry **updates itself** based on your actual usage:

- **usageCount** increments each time a model is used in a session
- **lastUsed** tracks when the model was last active
- Models unused for **90+ days** get flagged as stale in `/bubbles.retro` output
- When you start using a **new model** not in the registry, the agent prompts: *"You're using {model} which isn't in your model registry. Add it?"*
- **User decides** — agents suggest, never auto-modify

This means your registry stays current without manual maintenance. Just use models and the registry follows.

## Commands

```
# Enable cross-model review for a workflow run
/bubbles.workflow specs/042-feature mode: full-delivery crossModelReview: codex

# Just run a cross-model code review directly
/bubbles.code-review crossModelReview: codex
```

## What You Get

- Independent review from a different AI (different training, different blind spots)
- Cross-model finding comparison: overlapping vs. unique findings
- Higher confidence on findings both models agree on
- Novel findings neither model would catch alone

## Registry Freshness

Two freshness mechanisms work together:

1. **Usage-based** (`autoTrackUsage`): Models you stop using get flagged automatically. `/bubbles.retro` shows which models are stale.
2. **Calendar-based** (`lastVerified`): After 90 days, `bubbles.super` reminds you to review the full registry — new models appear frequently, old ones get deprecated.

Run `/bubbles.super update model registry` to refresh.

## Tips

- Overlapping findings are high-confidence — both models independently flagged the same issue
- Unique findings are where the value is — each model has different blind spots
- Use `adversarial` role for models that should try to break your code, not just review it
- Cross-model review is optional and additive — it never replaces the primary review
- The registry tracks usage automatically — just use models and it stays current
- When you switch to a new model mid-session, accept the prompt to add it to the registry
