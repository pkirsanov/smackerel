# Recipe: Security Review

> *"Safety... always ON."*

---

## The Situation

You need to check for security vulnerabilities before shipping.

## The Command

```
/bubbles.security  review security for 042-catalog-assistant
```

## What Gets Checked

- OWASP Top 10 vulnerabilities
- SQL injection / XSS / command injection
- Authentication and authorization gaps
- Cryptographic issues
- Insecure dependencies
- Secrets in code
- SSRF/CSRF risks
- Input validation gaps

## After Security Scan

Fix findings, then validate:

```
/bubbles.workflow  042-catalog-assistant mode: full-delivery
```
