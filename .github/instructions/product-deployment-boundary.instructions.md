---
applyTo: "**"
---

# Product Deployment Boundary (NON-NEGOTIABLE)

This product repository is target-agnostic. It may contain only generic deployment contracts, environment-variable seams, and placeholder-only examples.

ALL concrete deployment details belong exclusively in the operator-owned knb repository, including:

- deployment target and host names
- IP addresses, FQDNs, tailnet identities, and operator account names
- operator home/check-out paths
- target-specific port blocks, subnets, listeners, and deployment order
- reverse-proxy fragments, firewall rules, init-system units, and host singleton configuration
- concrete target params, manifests, adapter scripts, and target runbooks
- product-local skills or docs that reveal a concrete target

Historical reports, tests, comments, screenshots, generated logs, and examples are not exceptions. Use role placeholders such as `<deploy-host>`, `<host-tailnet-ip>`, `<tailnet>`, `<operator>`, `<operator-home>`, and `<knb-repo>`.

The knb-owned `scripts/lint/product-deployment-boundary.sh` scanner is blocking in pre-push and CI. It is fail-closed: if knb or the scanner is unavailable, the push must be refused. There is no bypass.
